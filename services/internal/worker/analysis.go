package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/analysis"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/safearchive"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/safefetch"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
)

const analysisTimeout = 10 * time.Minute

// userError is an error whose message is safe to show to users.
type userError struct{ msg string }

func (e userError) Error() string { return e.msg }

func (w *Worker) processAnalysisRun(parent context.Context, run store.AnalysisRun) {
	ctx, cancel := context.WithTimeout(parent, analysisTimeout)
	defer cancel()
	log := w.log.With(slog.String("run_id", run.ID.String()), slog.String("analyzer", run.Analyzer))
	start := time.Now()
	res, err := w.executeAnalysis(ctx, run)
	if err != nil {
		msg := "the analysis failed"
		var ue userError
		switch {
		case errors.As(err, &ue):
			msg = ue.msg
		case errors.Is(err, context.DeadlineExceeded):
			msg = "the analysis took longer than the time limit"
		case parent.Err() != nil:
			// Shutting down: put the run back so another worker can pick it up.
			_ = w.db.FinishAnalysisRun(context.Background(), store.FinishAnalysisRunParams{ID: run.ID, Status: "queued", Stats: json.RawMessage("{}")})
			return
		default:
			msg = "the analysis failed: " + err.Error()
		}
		log.Warn("analysis run failed", slog.String("error", err.Error()))
		if run.SourceID != nil {
			_ = w.db.SetAnalysisSourceStatus(context.Background(), store.SetAnalysisSourceStatusParams{ID: *run.SourceID, Status: "failed", Error: truncate(msg, 1000)})
		}
		_ = w.db.FinishAnalysisRun(context.Background(), store.FinishAnalysisRunParams{ID: run.ID, Status: "failed", Stats: json.RawMessage("{}"), Error: truncate(msg, 2000)})
		return
	}
	if res.Stats == nil {
		res.Stats = map[string]any{}
	}
	res.Stats["findings"] = len(res.Findings)
	res.Stats["duration_ms"] = time.Since(start).Milliseconds()
	stats, _ := json.Marshal(res.Stats)
	err = w.db.Tx(context.Background(), func(q *store.Queries, _ pgx.Tx) error {
		if run.SourceID != nil {
			// Re-running an analyzer replaces its previous rule-based findings for the source.
			if err := q.DeleteRunFindings(ctx, store.DeleteRunFindingsParams{SourceID: run.SourceID, RunID: &run.ID}); err != nil {
				return err
			}
			if err := q.SetAnalysisSourceStatus(ctx, store.SetAnalysisSourceStatusParams{ID: *run.SourceID, Status: "ready"}); err != nil {
				return err
			}
		}
		for _, f := range res.Findings {
			data, _ := json.Marshal(f.Data)
			if _, err := q.CreateFinding(ctx, store.CreateFindingParams{ProjectID: run.ProjectID, RunID: &run.ID, SourceID: run.SourceID,
				Category: f.Category, Title: f.Title, Detail: f.Detail, Evidence: f.Evidence, EvidenceLocator: f.Locator,
				Classification: f.Classification, Confidence: decimal.NewFromFloat(f.Confidence).Round(3), Assumptions: f.Assumptions,
				Origin: "rule", Data: data, CreatedBy: run.CreatedBy}); err != nil {
				return err
			}
		}
		if err := q.FinishAnalysisRun(ctx, store.FinishAnalysisRunParams{ID: run.ID, Status: "succeeded", Stats: stats}); err != nil {
			return err
		}
		if err := q.TouchAnalysisProject(ctx, run.ProjectID); err != nil {
			return err
		}
		return q.InsertAuditLog(ctx, store.InsertAuditLogParams{ActorUserID: run.CreatedBy, ActorLabel: "worker", Action: "analysis.run_completed",
			EntityType: "analysis_project", EntityID: &run.ProjectID,
			Summary: fmt.Sprintf("%s analysis produced %d findings", run.Analyzer, len(res.Findings))})
	})
	if err != nil {
		log.Error("store analysis results", slog.String("error", err.Error()))
		_ = w.db.FinishAnalysisRun(context.Background(), store.FinishAnalysisRunParams{ID: run.ID, Status: "failed", Stats: json.RawMessage("{}"), Error: "results could not be saved"})
		return
	}
	log.Info("analysis run finished", slog.Int("findings", len(res.Findings)))
}

func (w *Worker) executeAnalysis(ctx context.Context, run store.AnalysisRun) (analysis.Result, error) {
	if run.SourceID == nil {
		return analysis.Result{}, userError{"the run has no source"}
	}
	src, err := w.db.GetAnalysisSource(ctx, *run.SourceID)
	if err != nil {
		return analysis.Result{}, userError{"the source no longer exists"}
	}
	an, ok := analysis.Get(run.Analyzer)
	if !ok {
		return analysis.Result{}, userError{"unknown analyzer " + run.Analyzer}
	}
	var meta map[string]any
	_ = json.Unmarshal(src.Metadata, &meta)
	in := analysis.Input{Kind: src.Kind, Title: src.Title, URL: deref(src.Url), MimeType: src.MimeType, Text: src.ContentText, Metadata: meta}

	// Server-side fetch of a public page, only when the operator enabled it.
	if src.Kind == "url" && meta["fetch"] == true && strings.TrimSpace(src.ContentText) == "" {
		if !w.cfg.ServerFetch {
			return analysis.Result{}, userError{"server-side fetching was disabled after this source was added"}
		}
		f := safefetch.New(5<<20, w.cfg.FetchAllowPrivate)
		res, err := f.Fetch(ctx, deref(src.Url))
		if err != nil {
			return analysis.Result{}, userError{"the page could not be fetched: " + err.Error()}
		}
		if res.StatusCode >= 400 {
			return analysis.Result{}, userError{fmt.Sprintf("the site answered with HTTP %d; REA does not bypass access restrictions", res.StatusCode)}
		}
		meta["fetched_at"] = time.Now().UTC().Format(time.RFC3339)
		meta["final_url"] = res.FinalURL
		mb, _ := json.Marshal(meta)
		text := string(res.Body)
		if err := w.db.UpdateAnalysisSourceContent(ctx, store.UpdateAnalysisSourceContentParams{ID: src.ID, ContentText: text,
			ContentSha256: sha256Hex(text), Metadata: mb, Status: "ready"}); err != nil {
			return analysis.Result{}, err
		}
		in.Text = text
		in.URL = res.FinalURL
	}

	switch src.Kind {
	case "repository_archive":
		path, err := w.attachmentPath(ctx, src)
		if err != nil {
			return analysis.Result{}, err
		}
		files, err := safearchive.Read(path, safearchive.Limits{MaxFiles: w.cfg.MaxArchiveFiles, MaxTotalBytes: w.cfg.MaxArchiveBytes,
			MaxFileBytes: 2 << 20, MaxRatio: 100}, analysis.WantFile)
		if err != nil {
			return analysis.Result{}, userError{"the archive was rejected: " + err.Error()}
		}
		for _, f := range files {
			in.Files = append(in.Files, analysis.File{Path: f.Path, Size: f.Size, Content: f.Content, Skipped: f.Skipped})
		}
	case "repository_path":
		root, _ := meta["path"].(string)
		if !w.repoPathAllowed(root) {
			return analysis.Result{}, userError{"the repository path is no longer allowed by REA_REPO_ROOTS"}
		}
		files, err := analysis.ReadDirectory(root, w.cfg.MaxArchiveFiles, w.cfg.MaxArchiveBytes, 2<<20)
		if err != nil {
			return analysis.Result{}, userError{"the repository could not be read: " + err.Error()}
		}
		in.Files = files
	case "screenshot":
		data, name, err := w.attachmentBytes(ctx, src, 30<<20)
		if err != nil {
			return analysis.Result{}, err
		}
		in.RawFile, in.RawFilename = data, name
	case "document":
		if src.AttachmentID != nil {
			_, name, _ := w.attachmentBytes(ctx, src, 1)
			in.RawFilename = name
		}
	}
	if len(in.Files) > 0 {
		w.recordFileListing(ctx, src.ID, in.Files)
	}
	return an.Analyze(ctx, in)
}

// fileEntry is one line of the file listing shown in the codebase viewer.
type fileEntry struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	Skipped string `json:"skipped,omitempty"`
}

// recordFileListing stores the paths and sizes of a codebase source so the dashboard can show
// its file tree. File contents are never stored here. Failures only cost the listing.
func (w *Worker) recordFileListing(ctx context.Context, id uuid.UUID, files []analysis.File) {
	const maxEntries = 20000
	list := make([]fileEntry, 0, min(len(files), maxEntries))
	for _, f := range files {
		if len(list) == maxEntries {
			break
		}
		list = append(list, fileEntry{Path: f.Path, Size: f.Size, Skipped: f.Skipped})
	}
	b, err := json.Marshal(list)
	if err != nil {
		return
	}
	if err := w.db.SetAnalysisSourceFiles(ctx, store.SetAnalysisSourceFilesParams{ID: id, Files: b, FileCount: int32(len(files))}); err != nil {
		w.log.Warn("could not record file listing", slog.String("source_id", id.String()), slog.String("error", err.Error()))
	}
}

func (w *Worker) attachmentPath(ctx context.Context, src store.AnalysisSource) (string, error) {
	if src.AttachmentID == nil {
		return "", userError{"the source has no uploaded file"}
	}
	a, err := w.db.GetAttachment(ctx, *src.AttachmentID)
	if err != nil {
		return "", userError{"the uploaded file is missing"}
	}
	return w.files.Path(a.StorageKey)
}

func (w *Worker) attachmentBytes(ctx context.Context, src store.AnalysisSource, max int64) ([]byte, string, error) {
	if src.AttachmentID == nil {
		return nil, "", userError{"the source has no uploaded file"}
	}
	a, err := w.db.GetAttachment(ctx, *src.AttachmentID)
	if err != nil {
		return nil, "", userError{"the uploaded file is missing"}
	}
	f, err := w.files.Open(a.StorageKey)
	if err != nil {
		return nil, a.Filename, userError{"the uploaded file is missing"}
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, max))
	return data, a.Filename, err
}

func (w *Worker) repoPathAllowed(p string) bool {
	if p == "" {
		return false
	}
	for _, root := range w.cfg.RepoRoots {
		if p == root || strings.HasPrefix(p, root+"/") {
			return true
		}
	}
	return false
}
