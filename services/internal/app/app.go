// Package app contains start-up code shared by the API and worker binaries.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/config"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/migrate"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
)

// NewLogger builds the structured logger configured by REA_LOG_LEVEL and REA_LOG_FORMAT.
func NewLogger(level, format string) *slog.Logger {
	var lv slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lv = slog.LevelDebug
	case "warn", "warning":
		lv = slog.LevelWarn
	case "error":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: lv}
	if strings.ToLower(format) == "text" {
		return slog.New(slog.NewTextHandler(os.Stderr, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stderr, opts))
}

// OpenDatabase connects and optionally applies migrations.
func OpenDatabase(ctx context.Context, cfg *config.Config, log *slog.Logger, applyMigrations bool) (*store.DB, error) {
	if applyMigrations {
		v, err := migrate.Up(ctx, cfg.DatabaseURL)
		if err != nil {
			return nil, err
		}
		log.Info("database schema is current", slog.Int64("version", v))
	} else {
		st, err := migrate.GetStatus(ctx, cfg.DatabaseURL)
		if err != nil {
			return nil, err
		}
		if st.Pending > 0 {
			return nil, fmt.Errorf("%d database migrations are pending; run `rea-api migrate up` or set REA_AUTO_MIGRATE=true", st.Pending)
		}
	}
	return store.Open(ctx, cfg.DatabaseURL)
}
