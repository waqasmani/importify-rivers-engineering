// Package worker processes queued analysis runs and AI jobs. It runs embedded in the API process
// or standalone (services/worker). Jobs are claimed with FOR UPDATE SKIP LOCKED, so several workers
// can run against the same database safely.
package worker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/config"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/filestore"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/secretbox"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/webhooks"
)

// Worker claims and executes jobs.
type Worker struct {
	cfg      *config.Config
	db       *store.DB
	log      *slog.Logger
	box      *secretbox.Box
	files    *filestore.Store
	id       string
	wake     chan struct{}
	hooks    *webhooks.Dispatcher
	hookWake chan struct{}
}

// New creates a worker with a unique identifier used for job locks.
func New(cfg *config.Config, db *store.DB, log *slog.Logger, box *secretbox.Box, files *filestore.Store) *Worker {
	host, _ := os.Hostname()
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	id := host + "-" + hex.EncodeToString(b)
	return &Worker{cfg: cfg, db: db, log: log.With(slog.String("component", "worker")), box: box, files: files,
		id: id, wake: make(chan struct{}, 1), hooks: webhooks.New(cfg, db, box, log, id), hookWake: make(chan struct{}, 1)}
}

// Wake asks idle loops to check the queues immediately.
func (w *Worker) Wake() {
	for _, ch := range []chan struct{}{w.wake, w.hookWake} {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// webhookPoll is how often idle webhook loops look for new catalog changes and due retries.
const webhookPoll = 2 * time.Second

// staleAfter is how long a claimed job may run before another worker may requeue it.
const staleAfter = 30 * time.Minute

// Run processes jobs until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	w.log.Info("worker started", slog.String("id", w.id), slog.Int("concurrency", w.cfg.WorkerConcurrency))
	var wg sync.WaitGroup
	for i := 0; i < w.cfg.WorkerConcurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.loop(ctx)
		}()
	}
	// Webhook deliveries wait on remote servers, so they get their own loops and never hold up
	// analysis runs or AI jobs.
	for i := 0; i < w.cfg.WebhookConcurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.webhookLoop(ctx)
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		w.housekeeping(ctx)
	}()
	wg.Wait()
	w.log.Info("worker stopped")
}

func (w *Worker) webhookLoop(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		if w.hooks.RunOnce(ctx) {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-w.hookWake:
		case <-time.After(webhookPoll):
		}
	}
}

// DrainWebhooks sequences catalog changes, fans them out and sends every delivery that is due,
// until nothing is left (used by tests and the CLI).
func (w *Worker) DrainWebhooks(ctx context.Context) {
	for w.hooks.RunOnce(ctx) {
	}
}

func (w *Worker) loop(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		did := w.RunOnce(ctx)
		if did {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-w.wake:
		case <-time.After(5 * time.Second):
		}
	}
}

// RunOnce processes at most one analysis run and one AI job. It reports whether any work was done.
// Tests call it directly for deterministic processing.
func (w *Worker) RunOnce(ctx context.Context) bool {
	did := false
	if run, err := w.db.ClaimAnalysisRun(ctx, w.id); err == nil {
		did = true
		w.processAnalysisRun(ctx, run)
	} else if !store.IsNotFound(err) && ctx.Err() == nil {
		w.log.Error("claim analysis run", slog.String("error", err.Error()))
	}
	if job, err := w.db.ClaimAIJob(ctx, w.id); err == nil {
		did = true
		w.processAIJob(ctx, job)
	} else if !store.IsNotFound(err) && ctx.Err() == nil {
		w.log.Error("claim ai job", slog.String("error", err.Error()))
	}
	return did
}

// Drain runs RunOnce until the queues are empty (used by tests and the CLI).
func (w *Worker) Drain(ctx context.Context) {
	for w.RunOnce(ctx) {
	}
}

func (w *Worker) housekeeping(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		w.requeueStale(ctx)
		w.applyRetention(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (w *Worker) requeueStale(ctx context.Context) {
	if n, err := w.db.RequeueStaleAnalysisRuns(ctx, int32(staleAfter.Seconds())); err == nil && n > 0 {
		w.log.Warn("requeued stale analysis runs", slog.Int64("count", n))
	}
	if n, err := w.db.RequeueStaleAIJobs(ctx, int32(staleAfter.Seconds())); err == nil && n > 0 {
		w.log.Warn("requeued stale ai jobs", slog.Int64("count", n))
	}
	if n, err := w.db.RequeueStaleWebhookDeliveries(ctx, int32(webhooks.StaleAfter.Seconds())); err == nil && n > 0 {
		w.log.Warn("requeued stale webhook deliveries", slog.Int64("count", n))
	}
}
