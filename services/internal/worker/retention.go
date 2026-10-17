package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
)

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

var (
	retentionMu  sync.Mutex
	lastSweep    time.Time
	lastFileScan time.Time
)

// applyRetention deletes captured pages past the configured retention period (hourly) and removes
// attachment files that no database row references any more (daily).
func (w *Worker) applyRetention(ctx context.Context) {
	retentionMu.Lock()
	defer retentionMu.Unlock()
	if time.Since(lastSweep) < time.Hour {
		return
	}
	lastSweep = time.Now()
	if row, err := w.db.GetSetting(ctx, "capture"); err == nil {
		var cs struct {
			RetentionDays int `json:"retention_days"`
		}
		if json.Unmarshal(row.Value, &cs) == nil && cs.RetentionDays > 0 {
			before := time.Now().Add(-time.Duration(cs.RetentionDays) * 24 * time.Hour)
			if n, err := w.db.PurgeCapturedPagesBefore(ctx, before); err == nil && n > 0 {
				w.log.Info("purged captured pages past retention", slog.Int64("count", n))
				_ = w.db.InsertAuditLog(ctx, store.InsertAuditLogParams{ActorLabel: "worker", Action: "capture.retention_purge",
					EntityType: "captured_page", Summary: "Deleted captured pages older than the retention period"})
			}
		}
	}
	if time.Since(lastFileScan) < 24*time.Hour {
		return
	}
	lastFileScan = time.Now()
	w.collectOrphanFiles(ctx)
}

func (w *Worker) collectOrphanFiles(ctx context.Context) {
	keys, err := w.files.Keys()
	if err != nil {
		return
	}
	used, err := w.db.ListAllStorageKeys(ctx)
	if err != nil {
		return
	}
	inUse := map[string]bool{}
	for _, k := range used {
		inUse[k] = true
	}
	removed := 0
	for _, k := range keys {
		if !inUse[k] {
			if err := w.files.Delete(k); err == nil {
				removed++
			}
		}
	}
	if removed > 0 {
		w.log.Info("removed unreferenced attachment files", slog.Int("count", removed))
	}
}

// CollectOrphanFiles is exported for tests and operator commands.
func (w *Worker) CollectOrphanFiles(ctx context.Context) { w.collectOrphanFiles(ctx) }
