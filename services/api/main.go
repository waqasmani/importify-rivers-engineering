// Command rea-api runs the REA HTTP API and provides operator commands.
//
//	rea-api serve                 start the HTTP server (default)
//	rea-api migrate up|down|status
//	rea-api reset-password EMAIL  read a new password from stdin and revoke the user's sessions
//	rea-api healthcheck           exit 0 when the local server answers /healthz (for containers)
//	rea-api version
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/app"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/auth"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/config"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/httpapi"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/migrate"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/worker"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	var err error
	switch cmd {
	case "serve":
		err = serve()
	case "migrate":
		err = migrateCmd(os.Args[2:])
	case "reset-password":
		err = resetPassword(os.Args[2:])
	case "healthcheck":
		err = healthcheck()
	case "version":
		fmt.Println(version)
	default:
		err = fmt.Errorf("unknown command %q (expected serve, migrate, reset-password, healthcheck or version)", cmd)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func loadConfig() (*config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	if cfg.Version == "dev" {
		cfg.Version = version
	}
	return cfg, nil
}

func serve() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	log := app.NewLogger(cfg.LogLevel, cfg.LogFormat)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := app.OpenDatabase(ctx, cfg, log, cfg.AutoMigrate)
	if err != nil {
		return err
	}
	defer db.Close()

	var w *worker.Worker
	deps := httpapi.Deps{}
	srv, err := httpapi.New(cfg, db, log, deps)
	if err != nil {
		return err
	}
	if cfg.WorkerMode == "embedded" {
		w = worker.New(cfg, db, log, srv.Box(), srv.Files())
		srv.SetWake(w.Wake)
	}
	if err := srv.PrepareSetup(ctx); err != nil {
		return err
	}

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    64 << 10,
	}
	bg, cancelBG := context.WithCancel(context.Background())
	defer cancelBG()
	go srv.RunMaintenance(bg)
	workerDone := make(chan struct{})
	if w != nil {
		go func() { w.Run(bg); close(workerDone) }()
	} else {
		close(workerDone)
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("REA API listening", slog.String("addr", cfg.HTTPAddr), slog.String("public_url", cfg.PublicURL.String()),
			slog.String("version", cfg.Version), slog.String("worker", cfg.WorkerMode))
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil {
			return err
		}
	}
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Error("http shutdown", slog.String("error", err.Error()))
	}
	cancelBG()
	select {
	case <-workerDone:
	case <-shutdownCtx.Done():
		log.Warn("worker did not stop in time")
	}
	log.Info("stopped")
	return nil
}

func migrateCmd(args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	ctx := context.Background()
	sub := "up"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "up":
		v, err := migrate.Up(ctx, cfg.DatabaseURL)
		if err != nil {
			return err
		}
		fmt.Println("schema version", v)
	case "down":
		v, err := migrate.Down(ctx, cfg.DatabaseURL)
		if err != nil {
			return err
		}
		fmt.Println("schema version", v)
	case "status":
		st, err := migrate.GetStatus(ctx, cfg.DatabaseURL)
		if err != nil {
			return err
		}
		fmt.Printf("current %d, latest %d, pending %d\n", st.Current, st.Latest, st.Pending)
	default:
		return fmt.Errorf("unknown migrate command %q", sub)
	}
	return nil
}

func resetPassword(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: rea-api reset-password EMAIL  (the new password is read from standard input)")
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	ctx := context.Background()
	db, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	user, err := db.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(args[0])))
	if err != nil {
		return fmt.Errorf("user not found: %w", err)
	}
	fmt.Fprint(os.Stderr, "New password: ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return err
	}
	pw := strings.TrimRight(line, "\r\n")
	if msg := auth.ValidatePassword(pw); msg != "" {
		return errors.New("password " + msg)
	}
	hash, err := auth.HashPassword(pw)
	if err != nil {
		return err
	}
	if err := db.UpdateUserPassword(ctx, store.UpdateUserPasswordParams{ID: user.ID, PasswordHash: hash}); err != nil {
		return err
	}
	if err := db.RevokeUserSessions(ctx, user.ID); err != nil {
		return err
	}
	_ = db.InsertAuditLog(ctx, store.InsertAuditLogParams{Action: "user.reset_password_cli", EntityType: "user", EntityID: &user.ID,
		ActorLabel: "command line", Summary: "Password reset from the server command line"})
	fmt.Fprintln(os.Stderr, "Password updated and existing sessions revoked.")
	return nil
}

func healthcheck() error {
	addr := os.Getenv("REA_HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + addr + "/healthz")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unhealthy: %s", resp.Status)
	}
	return nil
}
