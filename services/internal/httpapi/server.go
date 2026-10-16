// Package httpapi implements the REA HTTP API.
package httpapi

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/config"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/filestore"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/ratelimit"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/secretbox"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
)

// Server holds the dependencies shared by every handler.
type Server struct {
	cfg   *config.Config
	db    *store.DB
	log   *slog.Logger
	box   *secretbox.Box
	files *filestore.Store
	deps  Deps

	limMu    sync.Mutex
	limiters []*ratelimit.Limiter

	setupMu    sync.Mutex
	setupToken string
}

// Deps are optional subsystems wired in by the composition root.
type Deps struct {
	// Wake is called after jobs are enqueued so an embedded worker can start immediately.
	Wake func()
}

// New builds a Server.
func New(cfg *config.Config, db *store.DB, log *slog.Logger, deps Deps) (*Server, error) {
	box, err := secretbox.New(cfg.SecretKey)
	if err != nil {
		return nil, err
	}
	files, err := filestore.New(filepath.Join(cfg.DataDir, "attachments"))
	if err != nil {
		return nil, err
	}
	if deps.Wake == nil {
		deps.Wake = func() {}
	}
	return &Server{cfg: cfg, db: db, log: log, box: box, files: files, deps: deps}, nil
}

// Box exposes the secret box for other subsystems (the AI worker decrypts provider keys).
func (s *Server) Box() *secretbox.Box { return s.box }

// Files exposes the attachment store.
func (s *Server) Files() *filestore.Store { return s.files }

func (s *Server) newLimiter(perMinute float64, burst int) *ratelimit.Limiter {
	l := ratelimit.New(perMinute, burst)
	s.limMu.Lock()
	s.limiters = append(s.limiters, l)
	s.limMu.Unlock()
	return l
}

// RunMaintenance periodically evicts idle rate-limit keys and expired sessions.
func (s *Server) RunMaintenance(ctx context.Context) {
	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.limMu.Lock()
			for _, l := range s.limiters {
				l.Sweep(15 * time.Minute)
			}
			s.limMu.Unlock()
			if n, err := s.db.DeleteExpiredSessions(ctx); err == nil && n > 0 {
				s.log.Info("removed expired sessions", slog.Int64("count", n))
			}
			_, _ = s.db.DeleteExpiredCredentials(ctx)
		}
	}
}

// Routes returns the complete HTTP handler.
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(s.requestContext, s.recoverPanics, s.logRequests, s.securityHeaders)

	r.Get("/healthz", s.h(s.handleHealth))
	r.Get("/readyz", s.h(s.handleReady))

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(s.cors, s.rateLimit(1200, 200), s.authenticate)
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			s.writeError(w, r, &APIError{Status: http.StatusNotFound, Code: "not_found", Message: "No such API endpoint."})
		})
		r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
			s.writeError(w, r, &APIError{Status: http.StatusMethodNotAllowed, Code: "method_not_allowed", Message: "Method not allowed."})
		})

		// Public endpoints with strict rate limits.
		r.Group(func(r chi.Router) {
			r.Use(s.rateLimit(20, 10))
			r.Get("/setup/status", s.h(s.handleSetupStatus))
			r.Post("/setup", s.h(s.handleSetup))
			r.Post("/auth/login", s.h(s.handleLogin))
			r.Post("/extension/pair", s.h(s.handleExtensionPair))
			r.Post("/extension/token/refresh", s.h(s.handleExtensionRefresh))
		})

		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			s.mountIdentity(r)
			for _, mount := range s.mounts() {
				mount(r)
			}
		})
	})

	if s.cfg.WebDir != "" {
		r.NotFound(s.spaHandler(s.cfg.WebDir))
	}
	return r
}

// spaHandler serves the built web application and falls back to index.html for client routes.
func (s *Server) spaHandler(dir string) http.HandlerFunc {
	root := os.DirFS(dir)
	fileServer := http.FileServerFS(root)
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			s.writeError(w, r, errNotFound("Endpoint"))
			return
		}
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if st, err := fs.Stat(root, p); err != nil || st.IsDir() {
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFileFS(w, r, root, "index.html")
			return
		}
		if strings.HasPrefix(p, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		fileServer.ServeHTTP(w, r)
	}
}

// SetWake registers the function that wakes an embedded worker after jobs are enqueued.
func (s *Server) SetWake(fn func()) { s.deps.Wake = fn }
