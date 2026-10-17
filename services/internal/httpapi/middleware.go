package httpapi

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/auth"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
)

const (
	sessionCookieSecure   = "__Host-rea_session"
	sessionCookieInsecure = "rea_session"
	csrfHeader            = "X-CSRF-Token"
)

func (s *Server) sessionCookieName() string {
	if s.cfg.CookieSecure {
		return sessionCookieSecure
	}
	return sessionCookieInsecure
}

// requestContext assigns a request ID and resolves the client IP.
func (s *Server) requestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if len(id) < 8 || len(id) > 64 || strings.ContainsFunc(id, func(c rune) bool {
			return !(c == '-' || c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'))
		}) {
			b := make([]byte, 8)
			_, _ = rand.Read(b)
			id = hex.EncodeToString(b)
		}
		w.Header().Set("X-Request-ID", id)
		ctx := r.Context()
		ctx = contextWith(ctx, requestIDKey, id)
		ctx = contextWith(ctx, clientIPKey, s.resolveClientIP(r))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// resolveClientIP trusts X-Forwarded-For only when the direct peer is a configured proxy.
func (s *Server) resolveClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if !s.cfg.IsTrustedProxy(r.RemoteAddr) {
		return host
	}
	parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(parts) - 1; i >= 0; i-- {
		ip := strings.TrimSpace(parts[i])
		if ip == "" {
			continue
		}
		if net.ParseIP(ip) == nil {
			break
		}
		if !s.cfg.IsTrustedProxy(ip) {
			return ip
		}
	}
	return host
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (sr *statusRecorder) WriteHeader(code int) {
	if sr.status == 0 {
		sr.status = code
	}
	sr.ResponseWriter.WriteHeader(code)
}

func (sr *statusRecorder) Write(b []byte) (int, error) {
	if sr.status == 0 {
		sr.status = http.StatusOK
	}
	n, err := sr.ResponseWriter.Write(b)
	sr.bytes += n
	return n, err
}

func (sr *statusRecorder) Unwrap() http.ResponseWriter { return sr.ResponseWriter }

// logRequests writes one structured line per request. Query strings, bodies and credentials are
// never logged.
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sr := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(sr, r)
		route := r.URL.Path
		if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
			route = rc.RoutePattern()
		}
		if route == "/healthz" || route == "/readyz" {
			return
		}
		attrs := []slog.Attr{
			slog.String("method", r.Method), slog.String("route", route), slog.Int("status", sr.status),
			slog.Int("bytes", sr.bytes), slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			slog.String("request_id", requestID(r)), slog.String("ip", clientIP(r)),
		}
		if p := principal(r); p != nil && p.Integration != nil {
			attrs = append(attrs, slog.String("integration_key_id", p.Integration.ID.String()), slog.String("auth", p.Method))
		} else if p != nil {
			attrs = append(attrs, slog.String("user_id", p.User.ID.String()), slog.String("auth", p.Method))
		}
		level := slog.LevelInfo
		if sr.status >= 500 {
			level = slog.LevelError
		}
		s.log.LogAttrs(r.Context(), level, "http request", attrs...)
	})
}

// recoverPanics converts panics into 500 responses.
func (s *Server) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				s.log.Error("panic", slog.Any("value", v), slog.String("request_id", requestID(r)), slog.String("stack", string(debug.Stack())))
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": &APIError{Code: "internal_error",
					Message: "An unexpected error occurred. Reference: " + requestID(r)}})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// securityHeaders sets production-appropriate response headers.
func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		} else {
			h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data: blob: https: http:; font-src 'self' data:; connect-src 'self'; object-src 'none'; "+
				"frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		}
		if s.cfg.CookieSecure {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// cors allows credentialed requests from configured web origins and bearer-token requests from
// configured extension origins. Other origins receive no CORS headers, so browsers block them.
func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			h := w.Header()
			h.Add("Vary", "Origin")
			allowed := false
			if slices.Contains(s.cfg.AllowedOrigins, origin) {
				allowed = true
				h.Set("Access-Control-Allow-Credentials", "true")
			} else if slices.Contains(s.cfg.ExtensionOrigins, origin) {
				allowed = true
			}
			if allowed {
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Access-Control-Expose-Headers", "X-Request-ID, Content-Disposition, ETag")
				if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
					h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE")
					h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-CSRF-Token, X-Request-ID, X-REA-Extension-Version, If-None-Match")
					h.Set("Access-Control-Max-Age", "600")
					w.WriteHeader(http.StatusNoContent)
					return
				}
			} else if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// rateLimit applies a per-IP limiter.
func (s *Server) rateLimit(perMinute float64, burst int) func(http.Handler) http.Handler {
	lim := s.newLimiter(perMinute, burst)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !lim.Allow(clientIP(r)) {
				w.Header().Set("Retry-After", "60")
				writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": &APIError{Code: "rate_limited",
					Message: "Too many requests. Wait a minute and try again."}})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// authenticate resolves a bearer token or session cookie into a Principal. It does not reject
// anonymous requests; requireAuth does.
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := s.resolvePrincipal(r)
		if err != nil {
			if ae, isAPI := err.(*APIError); isAPI && ae.Status == http.StatusTooManyRequests {
				w.Header().Set("Retry-After", "60")
			}
			s.writeError(w, r, err)
			return
		}
		if p != nil {
			r = r.WithContext(withPrincipal(r.Context(), p))
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) resolvePrincipal(r *http.Request) (*Principal, error) {
	ctx := r.Context()
	if authz := r.Header.Get("Authorization"); authz != "" {
		token, found := strings.CutPrefix(authz, "Bearer ")
		if found && strings.HasPrefix(token, auth.PrefixIntegration) {
			return s.resolveIntegrationKey(r, token)
		}
		if !found || !(strings.HasPrefix(token, auth.PrefixAccess) || strings.HasPrefix(token, auth.PrefixPAT)) {
			return nil, errUnauthorized("The access token is invalid.")
		}
		row, err := s.db.GetCredentialByHash(ctx, auth.HashToken(token))
		if store.IsNotFound(err) {
			return nil, &APIError{Status: http.StatusUnauthorized, Code: "token_expired",
				Message: "The access token is invalid, expired or revoked."}
		}
		if err != nil {
			return nil, err
		}
		if row.ApiCredential.Kind == "refresh" {
			return nil, errUnauthorized("Refresh tokens cannot be used to call the API.")
		}
		if !row.User.IsActive {
			return nil, errUnauthorized("This account is disabled.")
		}
		rolePerms, err := s.db.PermissionsForRole(ctx, row.User.RoleID)
		if err != nil {
			return nil, err
		}
		perms := map[string]bool{}
		for _, code := range rolePerms {
			if slices.Contains(row.ApiCredential.Scopes, code) {
				perms[code] = true
			}
		}
		auth.AddCatalogPermissions(perms)
		c := row.ApiCredential
		if c.LastUsedAt == nil || time.Since(*c.LastUsedAt) > time.Minute {
			_ = s.db.TouchCredential(ctx, c.ID)
			if c.DeviceID != nil {
				_ = s.db.TouchExtensionDevice(ctx, store.TouchExtensionDeviceParams{ID: *c.DeviceID, LastIp: clientIP(r),
					Version: truncateHeader(r.Header.Get("X-REA-Extension-Version"), 40)})
			}
		}
		return &Principal{User: row.User, Perms: perms, Method: "token", CredentialID: c.ID, DeviceID: c.DeviceID}, nil
	}
	cookie, err := r.Cookie(s.sessionCookieName())
	if err != nil || !strings.HasPrefix(cookie.Value, auth.PrefixSession) {
		return nil, nil
	}
	row, err := s.db.GetActiveSessionByHash(ctx, auth.HashToken(cookie.Value))
	if store.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sess := row.UserSession
	if !row.User.IsActive {
		_ = s.db.RevokeSession(ctx, sess.ID)
		return nil, nil
	}
	if time.Since(sess.LastSeenAt) > s.cfg.SessionIdle {
		_ = s.db.RevokeSession(ctx, sess.ID)
		return nil, nil
	}
	if time.Since(sess.LastSeenAt) > 5*time.Minute {
		_ = s.db.TouchSession(ctx, sess.ID)
	}
	rolePerms, err := s.db.PermissionsForRole(ctx, row.User.RoleID)
	if err != nil {
		return nil, err
	}
	perms := map[string]bool{}
	for _, code := range rolePerms {
		perms[code] = true
	}
	auth.AddCatalogPermissions(perms)
	return &Principal{User: row.User, Perms: perms, Method: "session", SessionID: sess.ID, CSRFToken: sess.CsrfToken}, nil
}

// resolveIntegrationKey authenticates an organization-owned integration key. Its permissions are
// exactly its scopes; no user role is involved. Each key is rate limited on its own, every request
// is counted per day, and the first use in each hour (or from a new address) is audited.
func (s *Server) resolveIntegrationKey(r *http.Request, token string) (*Principal, error) {
	ctx := r.Context()
	k, err := s.db.GetActiveIntegrationKeyByHash(ctx, auth.HashToken(token))
	if store.IsNotFound(err) {
		return nil, &APIError{Status: http.StatusUnauthorized, Code: "token_expired",
			Message: "The integration key is invalid, expired or revoked."}
	}
	if err != nil {
		return nil, err
	}
	if !s.integrationLimiter.Allow(k.ID.String()) {
		return nil, &APIError{Status: http.StatusTooManyRequests, Code: "rate_limited",
			Message: "This integration key sent too many requests. Wait a minute and try again."}
	}
	perms := map[string]bool{}
	for _, sc := range k.Scopes {
		if auth.IsIntegrationScope(sc) {
			perms[sc] = true
		}
	}
	p := &Principal{Perms: perms, Method: "integration", Integration: &IntegrationActor{ID: k.ID, Name: k.Name, Prefix: k.TokenPrefix}}
	if err := s.db.CountIntegrationKeyRequest(ctx, k.ID); err != nil {
		return nil, err
	}
	ip := clientIP(r)
	if k.LastUsedAt == nil || time.Since(*k.LastUsedAt) > time.Minute || k.LastUsedIp != ip || k.LastUsedAt.Hour() != time.Now().Hour() {
		first, err := s.db.TouchIntegrationKey(ctx, store.TouchIntegrationKeyParams{ID: k.ID, Ip: ip})
		if err != nil {
			return nil, err
		}
		if first {
			ar := r.WithContext(withPrincipal(ctx, p))
			_ = s.audit(ctx, nil, ar, auditEvent{Action: "integration_key.use", EntityType: "integration_key", EntityID: &k.ID,
				Summary: "Integration key " + k.Name + " used from " + ip, Extra: map[string]any{"method": r.Method, "path": r.URL.Path}})
		}
	}
	return p, nil
}

// integrationRouteAllowed lists the only endpoints integration keys may call: the catalog, product
// import submission and the order intake and automation endpoints. Everything else (CRM,
// administration, settings, profile, analysis, AI, confirming supplier orders) is refused before
// routing, in addition to the per-route permission checks.
func integrationRouteAllowed(method, path string) bool {
	if integrationOrderRouteAllowed(method, path) {
		return true
	}
	switch {
	case method == http.MethodGet || method == http.MethodHead:
		return strings.HasPrefix(path, "/api/v1/catalog/")
	case method == http.MethodPost:
		return path == "/api/v1/imports/products" || path == "/api/v1/imports/products/check"
	}
	return false
}

func truncateHeader(v string, n int) string {
	if len(v) > n {
		return v[:n]
	}
	return v
}

// requireAuth rejects anonymous requests and enforces CSRF tokens for cookie sessions.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := principal(r)
		if p == nil {
			s.writeError(w, r, errUnauthorized("Sign in to continue."))
			return
		}
		if p.Integration != nil && !integrationRouteAllowed(r.Method, r.URL.Path) {
			s.writeError(w, r, errForbidden("Integration keys can only call the catalog (/api/v1/catalog), product import and order intake and reporting endpoints."))
			return
		}
		if p.Method == "session" && !isSafeMethod(r.Method) {
			got := r.Header.Get(csrfHeader)
			if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(p.CSRFToken)) != 1 {
				s.writeError(w, r, &APIError{Status: http.StatusForbidden, Code: "csrf_failed",
					Message: "The request is missing a valid CSRF token. Reload the page and try again."})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// requirePerm rejects principals lacking every one of the listed permissions.
func (s *Server) requirePerm(perms ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := principal(r)
			for _, perm := range perms {
				if p.Can(perm) {
					next.ServeHTTP(w, r)
					return
				}
			}
			s.writeError(w, r, errForbidden("You do not have permission to do this ("+strings.Join(perms, " or ")+")."))
		})
	}
}

// requireSession restricts an endpoint to interactive sessions (not API tokens).
func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if principal(r).Method != "session" {
			s.writeError(w, r, errForbidden("This action is only available from the web application."))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isSafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

func (s *Server) can(r *http.Request, perm string) error {
	if !principal(r).Can(perm) {
		return errForbidden("You do not have permission to do this (" + perm + ").")
	}
	return nil
}

func uuidOrNil(p *uuid.UUID) uuid.UUID {
	if p == nil {
		return uuid.Nil
	}
	return *p
}
