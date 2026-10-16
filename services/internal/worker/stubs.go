package worker

import (
	"context"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
)

func (w *Worker) processAnalysisRun(ctx context.Context, run store.AnalysisRun) {
	_ = w.db.FinishAnalysisRun(ctx, store.FinishAnalysisRunParams{ID: run.ID, Status: "failed", Stats: []byte("{}"), Error: "analyzer not available yet"})
}

func (w *Worker) processAIJob(ctx context.Context, job store.AiJob) {}

func (w *Worker) applyRetention(ctx context.Context) {}
