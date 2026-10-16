package httpapi

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/auth"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
)

func (s *Server) mountSearch(r chi.Router) {
	r.Get("/search", s.h(s.handleSearch))
	r.Get("/dashboard", s.h(s.handleDashboard))
	r.With(s.requirePerm(auth.PermDataImport)).Post("/import/csv/preview", s.h(s.handleImportPreview))
	r.With(s.requirePerm(auth.PermDataImport)).Post("/import/csv/commit", s.h(s.handleImportCommit))
}

var searchPerms = map[string]string{
	"company": auth.PermCRMRead, "contact": auth.PermCRMRead, "supplier": auth.PermCRMRead, "lead": auth.PermCRMRead,
	"opportunity": auth.PermCRMRead, "task": auth.PermCRMRead, "product": auth.PermProductsRead,
	"captured_page": auth.PermProductsRead, "analysis_project": auth.PermAnalysisRead, "finding": auth.PermAnalysisRead,
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) error {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(q)) < 2 {
		return ok(w, map[string]any{"items": []any{}, "query": q})
	}
	if len(q) > 200 {
		q = q[:200]
	}
	rows, err := s.db.GlobalSearch(r.Context(), store.GlobalSearchParams{Q: q, PerType: 8})
	if err != nil {
		return err
	}
	p := principal(r)
	type hit struct {
		EntityType string    `json:"entity_type"`
		ID         uuid.UUID `json:"id"`
		Title      string    `json:"title"`
		Subtitle   string    `json:"subtitle"`
	}
	out := []hit{}
	for _, row := range rows {
		if p.Can(searchPerms[row.EntityType]) {
			out = append(out, hit{row.EntityType, row.ID, row.Title, row.Subtitle})
		}
	}
	return ok(w, map[string]any{"items": out, "query": q})
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) error {
	p := principal(r)
	counts, err := s.db.DashboardCounts(r.Context())
	if err != nil {
		return err
	}
	resp := map[string]any{}
	if p.Can(auth.PermCRMRead) || p.Can(auth.PermProductsRead) || p.Can(auth.PermAnalysisRead) {
		resp["counts"] = counts
	}
	if p.Can(auth.PermProductsRead) {
		perDay, err := s.db.ImportsPerDay(r.Context(), 30)
		if err != nil {
			return err
		}
		resp["imports_per_day"] = perDay
	}
	if p.Can(auth.PermCRMRead) {
		mine, err := s.db.ListTasks(r.Context(), store.ListTasksParams{AssigneeID: &p.User.ID, OpenOnly: boolPtr(true), Sort: "due", Lim: 10})
		if err != nil {
			return err
		}
		tasks := make([]taskView, 0, len(mine))
		for _, t := range mine {
			tasks = append(tasks, taskView{Task: t.Task, Overdue: taskOverdue(t.Task)})
		}
		resp["my_tasks"] = tasks
		if pl, err := s.db.GetDefaultPipeline(r.Context(), "opportunity"); err == nil {
			sum, err := s.db.PipelineSummary(r.Context(), pl.ID)
			if err != nil {
				return err
			}
			resp["pipeline"] = map[string]any{"pipeline": pl, "stages": sum}
		}
	}
	if p.Can(auth.PermAuditRead) {
		act, err := s.db.RecentActivity(r.Context(), 15)
		if err != nil {
			return err
		}
		resp["recent_activity"] = act
	} else {
		act, err := s.db.ListAuditLogs(r.Context(), store.ListAuditLogsParams{ActorUserID: &p.User.ID, Lim: 15})
		if err != nil {
			return err
		}
		resp["recent_activity"] = act
	}
	return ok(w, resp)
}

func boolPtr(b bool) *bool { return &b }
