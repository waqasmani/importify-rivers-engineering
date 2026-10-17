// Command rea-worker processes analysis runs and AI jobs outside the API process. It is optional:
// by default the API runs the same worker in-process (REA_WORKER_MODE=embedded).
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/app"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/config"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/filestore"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/secretbox"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/worker"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.Version == "dev" {
		cfg.Version = version
	}
	log := app.NewLogger(cfg.LogLevel, cfg.LogFormat)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	// The API owns migrations; the worker waits for a current schema.
	db, err := app.OpenDatabase(ctx, cfg, log, false)
	if err != nil {
		return err
	}
	defer db.Close()
	box, err := secretbox.New(cfg.SecretKey)
	if err != nil {
		return err
	}
	files, err := filestore.New(filepath.Join(cfg.DataDir, "attachments"))
	if err != nil {
		return err
	}
	worker.New(cfg, db, log, box, files).Run(ctx)
	return nil
}
