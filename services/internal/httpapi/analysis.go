package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/analysis"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/auth"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/normalize"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/valid"
)

var analysisTypes = []string{"website", "product", "codebase", "api", "document", "workflow", "reconstruction"}
var authorizationBases = []string{"owned", "client_authorized", "public_information", "licensed", "open_source", "other"}
var projectStatuses = []string{"draft", "active", "completed", "archived"}
var classifications = []string{"verified", "inferred", "unknown"}

func (s *Server) mountAnalysis(r chi.Router) {
	r.Route("/analysis", func(r chi.Router) {
		r.With(s.requirePerm(auth.PermAnalysisRead)).Get("/analyzers", s.h(s.handleListAnalyzers))
		r.With(s.requirePerm(auth.PermAnalysisRead)).Get("/projects", s.h(s.handleListProjects))
		r.With(s.requirePerm(auth.PermAnalysisWrite)).Post("/projects", s.h(s.handleCreateProject))
		r.With(s.requirePerm(auth.PermAnalysisRead)).Get("/projects/{id}", s.h(s.handleGetProject))
		r.With(s.requirePerm(auth.PermAnalysisWrite)).Put("/projects/{id}", s.h(s.handleUpdateProject))
		r.With(s.requirePerm(auth.PermAnalysisDelete)).Delete("/projects/{id}", s.h(s.handleDeleteProject))

		r.With(s.requirePerm(auth.PermAnalysisRead)).Get("/projects/{id}/sources", s.h(s.handleListSources))
		r.With(s.requirePerm(auth.PermAnalysisWrite)).Post("/projects/{id}/sources", s.h(s.handleCreateSource))
		r.With(s.requirePerm(auth.PermAnalysisWrite)).Post("/projects/{id}/sources/upload", s.h(s.handleUploadSource))
		r.With(s.requirePerm(auth.PermAnalysisRead)).Get("/sources/{sourceID}", s.h(s.handleGetSource))
		r.With(s.requirePerm(auth.PermAnalysisWrite)).Delete("/projects/{id}/sources/{sourceID}", s.h(s.handleDeleteSource))

		r.With(s.requirePerm(auth.PermAnalysisRead)).Get("/projects/{id}/runs", s.h(s.handleListRuns))
		r.With(s.requirePerm(auth.PermAnalysisWrite)).Post("/projects/{id}/runs", s.h(s.handleCreateRun))
		r.With(s.requirePerm(auth.PermAnalysisRead)).Get("/runs/{runID}", s.h(s.handleGetRun))
		r.With(s.requirePerm(auth.PermAnalysisWrite)).Post("/runs/{runID}/cancel", s.h(s.handleCancelRun))

		r.With(s.requirePerm(auth.PermAnalysisRead)).Get("/projects/{id}/findings", s.h(s.handleListFindings))
		r.With(s.requirePerm(auth.PermAnalysisRead)).Get("/projects/{id}/findings/summary", s.h(s.handleFindingSummary))
		r.With(s.requirePerm(auth.PermAnalysisWrite)).Post("/projects/{id}/findings", s.h(s.handleCreateFinding))
		r.With(s.requirePerm(auth.PermAnalysisRead)).Get("/findings/{findingID}", s.h(s.handleGetFinding))
		r.With(s.requirePerm(auth.PermAnalysisWrite)).Put("/findings/{findingID}", s.h(s.handleUpdateFinding))
		r.With(s.requirePerm(auth.PermAnalysisWrite)).Delete("/findings/{findingID}", s.h(s.handleDeleteFinding))

		r.With(s.requirePerm(auth.PermAnalysisRead)).Get("/projects/{id}/reports", s.h(s.handleListReports))
		r.With(s.requirePerm(auth.PermAnalysisWrite)).Post("/projects/{id}/reports", s.h(s.handleCreateReport))
		r.With(s.requirePerm(auth.PermAnalysisRead)).Get("/reports/{reportID}", s.h(s.handleGetReport))
		r.With(s.requirePerm(auth.PermAnalysisRead)).Get("/reports/{reportID}/export", s.h(s.handleExportReport))
		r.With(s.requirePerm(auth.PermAnalysisWrite)).Delete("/reports/{reportID}", s.h(s.handleDeleteReport))
	})
}

func (s *Server) handleListAnalyzers(w http.ResponseWriter, r *http.Request) error {
	return ok(w, map[string]any{
		"analyzers":        []string{"website", "codebase", "openapi", "api_examples", "document", "screenshot", "workflow"},
		"report_kinds":     analysis.ReportKinds,
		"source_kinds":     []string{"url", "captured_page", "html", "repository_archive", "repository_path", "openapi", "api_examples", "document", "screenshot", "text", "workflow"},
		"server_fetch":     s.cfg.ServerFetch,
		"repository_paths": len(s.cfg.RepoRoots) > 0,
	})
}

// ---- projects ----

type projectInput struct {
	WorkspaceID            uuid.UUID  `json:"workspace_id"`
	Name                   string     `json:"name"`
	Description            string     `json:"description"`
	Objective              string     `json:"objective"`
	Status                 string     `json:"status"`
	AnalysisTypes          []string   `json:"analysis_types"`
	AuthorizationBasis     string     `json:"authorization_basis"`
	AuthorizationNote      string     `json:"authorization_note"`
	AuthorizationConfirmed bool       `json:"authorization_confirmed"`
	OwnerID                *uuid.UUID `json:"owner_id"`
}

func (s *Server) validateProject(r *http.Request, in *projectInput, creating bool) error {
	in.Name = normalize.Text(in.Name)
	in.Description = normalize.MultilineText(in.Description)
	in.Objective = normalize.MultilineText(in.Objective)
	in.AuthorizationNote = normalize.MultilineText(in.AuthorizationNote)
	if in.Status == "" {
		in.Status = "active"
	}
	v := valid.New()
	v.Len("name", in.Name, 1, 200)
	v.Max("description", in.Description, 10000)
	v.Max("objective", in.Objective, 5000)
	v.OneOf("status", in.Status, projectStatuses...)
	for _, t := range in.AnalysisTypes {
		v.OneOf("analysis_types", t, analysisTypes...)
	}
	if creating {
		v.OneOf("authorization_basis", in.AuthorizationBasis, authorizationBases...)
		v.Check(in.AuthorizationConfirmed, "authorization_confirmed",
			"must be confirmed: you may only analyze systems and material you own or are authorized to examine")
		if in.AuthorizationBasis == "other" || in.AuthorizationBasis == "client_authorized" {
			v.Check(len(in.AuthorizationNote) >= 10, "authorization_note", "must describe who authorized the analysis")
		}
		v.Max("authorization_note", in.AuthorizationNote, 2000)
	}
	if _, err := s.db.GetWorkspace(r.Context(), in.WorkspaceID); err != nil {
		v.Add("workspace_id", "does not refer to an existing workspace")
	}
	s.checkUserRef(r.Context(), in.OwnerID, "owner_id", v)
	return checkValid(v)
}

type projectView struct {
	store.AnalysisProject
	WorkspaceName string `json:"workspace_name,omitempty"`
	SourceCount   int64  `json:"source_count"`
	FindingCount  int64  `json:"finding_count"`
	ReportCount   int64  `json:"report_count"`
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) error {
	limit, offset := pagination(r)
	ws, err := qUUID(r, "workspace_id")
	if err != nil {
		return err
	}
	rows, err := s.db.ListAnalysisProjects(r.Context(), store.ListAnalysisProjectsParams{Q: qText(r, "q"), WorkspaceID: ws, Status: qText(r, "status"), Lim: limit, Off: offset})
	if err != nil {
		return err
	}
	out := make([]projectView, 0, len(rows))
	var total int64
	for _, row := range rows {
		out = append(out, projectView{row.AnalysisProject, row.WorkspaceName, row.SourceCount, row.FindingCount, row.ReportCount})
		total = row.Total
	}
	return ok(w, Page[projectView]{Items: out, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) error {
	var in projectInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := s.validateProject(r, &in, true); err != nil {
		return err
	}
	if in.AnalysisTypes == nil {
		in.AnalysisTypes = []string{}
	}
	var p store.AnalysisProject
	err := s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		var err error
		p, err = q.CreateAnalysisProject(r.Context(), store.CreateAnalysisProjectParams{WorkspaceID: in.WorkspaceID, Name: in.Name,
			Description: in.Description, Objective: in.Objective, Status: in.Status, AnalysisTypes: in.AnalysisTypes,
			AuthorizationBasis: in.AuthorizationBasis, AuthorizationNote: in.AuthorizationNote, AuthorizationConfirmedBy: userIDPtr(r),
			OwnerID: in.OwnerID, CreatedBy: userIDPtr(r)})
		if err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "analysis.project_create", EntityType: "analysis_project", EntityID: &p.ID,
			Summary: "Created analysis project " + p.Name + " (authorization: " + p.AuthorizationBasis + ")", After: p})
	})
	if err != nil {
		return err
	}
	return created(w, p)
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	p, err := s.db.GetAnalysisProject(r.Context(), id)
	if err != nil {
		return err
	}
	ws, _ := s.db.GetWorkspace(r.Context(), p.WorkspaceID)
	counts, err := s.db.FindingCategoryCounts(r.Context(), id)
	if err != nil {
		return err
	}
	tags, err := s.recordTags(r.Context(), "analysis_project", id)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"project": p, "workspace": ws, "finding_counts": counts, "tags": tags})
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in projectInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := s.validateProject(r, &in, false); err != nil {
		return err
	}
	if in.AnalysisTypes == nil {
		in.AnalysisTypes = []string{}
	}
	var p store.AnalysisProject
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		before, err := q.GetAnalysisProject(r.Context(), id)
		if err != nil {
			return err
		}
		if p, err = q.UpdateAnalysisProject(r.Context(), store.UpdateAnalysisProjectParams{ID: id, WorkspaceID: in.WorkspaceID, Name: in.Name,
			Description: in.Description, Objective: in.Objective, Status: in.Status, AnalysisTypes: in.AnalysisTypes, OwnerID: in.OwnerID}); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "analysis.project_update", EntityType: "analysis_project", EntityID: &id,
			Summary: "Updated analysis project " + p.Name, Before: before, After: p})
	})
	if err != nil {
		return err
	}
	return ok(w, p)
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		before, err := q.GetAnalysisProject(r.Context(), id)
		if err != nil {
			return err
		}
		if _, err := q.DeleteAnalysisProject(r.Context(), id); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "analysis.project_delete", EntityType: "analysis_project", EntityID: &id,
			Summary: "Deleted analysis project " + before.Name + " with its sources, findings and reports", Before: before})
	})
	if err != nil {
		return err
	}
	return noContent(w)
}

// ---- sources ----

type sourceInput struct {
	Kind           string                  `json:"kind"`
	Title          string                  `json:"title"`
	URL            *string                 `json:"url"`
	Content        string                  `json:"content"`
	CapturedPageID *uuid.UUID              `json:"captured_page_id"`
	Path           string                  `json:"path"`
	Steps          []analysis.WorkflowStep `json:"steps"`
	Fetch          bool                    `json:"fetch"`
	AutoAnalyze    *bool                   `json:"auto_analyze"`
}

const maxSourceText = 5_000_000

// sensitiveKeyRe matches field names whose values are always redacted from recorded API traffic.
var sensitiveKeyRe = regexp.MustCompile(`(?i)(pass(word)?|secret|token|authorization|api[_-]?key|session|cookie|credential|private[_-]?key|otp)`)

func redactJSON(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, x := range t {
			if sensitiveKeyRe.MatchString(k) {
				t[k] = "[redacted]"
			} else {
				t[k] = redactJSON(x)
			}
		}
	case []any:
		for i, x := range t {
			t[i] = redactJSON(x)
		}
	}
	return v
}

// sanitizeExamples drops headers and redacts credentials from recorded API traffic before storage.
func sanitizeExamples(text string) (string, error) {
	var raw any
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return "", errField("content", "must be JSON: an array of examples or a HAR file")
	}
	stripHeaders := func(m map[string]any) {
		delete(m, "headers")
		delete(m, "cookies")
		if u, ok := m["url"].(string); ok {
			m["url"] = redactQuery(u)
		}
		if pd, ok := m["postData"].(map[string]any); ok {
			if t, ok := pd["text"].(string); ok {
				var body any
				if json.Unmarshal([]byte(t), &body) == nil {
					b, _ := json.Marshal(redactJSON(body))
					pd["text"] = string(b)
				} else {
					pd["text"] = "[non-JSON body removed]"
				}
			}
			delete(pd, "params")
		}
	}
	if m, ok := raw.(map[string]any); ok && m["log"] != nil {
		log, _ := m["log"].(map[string]any)
		entries, _ := log["entries"].([]any)
		for _, e := range entries {
			em, _ := e.(map[string]any)
			if req, ok := em["request"].(map[string]any); ok {
				stripHeaders(req)
			}
			if resp, ok := em["response"].(map[string]any); ok {
				delete(resp, "headers")
				delete(resp, "cookies")
				if c, ok := resp["content"].(map[string]any); ok {
					if t, ok := c["text"].(string); ok {
						var body any
						if json.Unmarshal([]byte(t), &body) == nil {
							b, _ := json.Marshal(redactJSON(body))
							c["text"] = string(b)
						} else {
							delete(c, "text")
						}
					}
				}
			}
		}
		b, _ := json.Marshal(map[string]any{"log": map[string]any{"version": log["version"], "entries": entries}})
		return string(b), nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return "", errField("content", "must be an array of examples or a HAR file")
	}
	for _, e := range arr {
		if m, ok := e.(map[string]any); ok {
			stripHeaders(m)
			m["request_body"] = redactJSON(m["request_body"])
			m["response_body"] = redactJSON(m["response_body"])
		}
	}
	b, _ := json.Marshal(arr)
	return string(b), nil
}

func redactQuery(u string) string {
	i := strings.IndexByte(u, '?')
	if i < 0 {
		return u
	}
	parts := strings.Split(u[i+1:], "&")
	for j, p := range parts {
		k, _, _ := strings.Cut(p, "=")
		if sensitiveKeyRe.MatchString(k) {
			parts[j] = k + "=[redacted]"
		}
	}
	return u[:i+1] + strings.Join(parts, "&")
}

// resolveRepoPath accepts only directories inside an operator-configured root, after resolving symlinks.
func (s *Server) resolveRepoPath(p string) (string, error) {
	if len(s.cfg.RepoRoots) == 0 {
		return "", errConflict("repository_paths_disabled", "Repository path analysis is disabled. The operator can allow directories with REA_REPO_ROOTS.")
	}
	abs, err := filepath.Abs(filepath.Clean(p))
	if err != nil {
		return "", errField("path", "is not a valid path")
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", errField("path", "does not exist on the server")
	}
	st, err := os.Stat(real)
	if err != nil || !st.IsDir() {
		return "", errField("path", "must be a directory")
	}
	for _, root := range s.cfg.RepoRoots {
		rr, err := filepath.EvalSymlinks(root)
		if err != nil {
			continue
		}
		if real == rr || strings.HasPrefix(real, rr+string(os.PathSeparator)) {
			return real, nil
		}
	}
	return "", errField("path", "is outside the directories the operator allowed (REA_REPO_ROOTS)")
}

func (s *Server) projectFor(r *http.Request) (store.AnalysisProject, error) {
	id, err := pathUUID(r, "id")
	if err != nil {
		return store.AnalysisProject{}, err
	}
	p, err := s.db.GetAnalysisProject(r.Context(), id)
	if err != nil {
		return p, errNotFound("Analysis project")
	}
	return p, nil
}

func (s *Server) handleCreateSource(w http.ResponseWriter, r *http.Request) error {
	project, err := s.projectFor(r)
	if err != nil {
		return err
	}
	var in sourceInput
	if err := decodeLimit(r, &in, 6<<20); err != nil {
		return err
	}
	in.Title = normalize.Text(in.Title)
	v := valid.New()
	v.OneOf("kind", in.Kind, "url", "captured_page", "html", "repository_path", "openapi", "api_examples", "text", "workflow")
	v.Max("title", in.Title, 300)
	v.Check(len(in.Content) <= maxSourceText, "content", "is larger than 5 MB")
	if err := checkValid(v); err != nil {
		return err
	}
	params := store.CreateAnalysisSourceParams{ProjectID: project.ID, Kind: in.Kind, Title: in.Title, Metadata: json.RawMessage("{}"),
		Status: "ready", CreatedBy: userIDPtr(r)}
	meta := map[string]any{}
	switch in.Kind {
	case "url":
		u := strings.TrimSpace(deref(in.URL))
		if !normalize.ValidHTTPURL(u) {
			return errField("url", "must be an absolute http or https URL")
		}
		params.Url = &u
		if in.Fetch {
			if !s.cfg.ServerFetch {
				return errConflict("server_fetch_disabled", "Server-side fetching is disabled. Capture the page with the browser extension, or ask the operator to set REA_SERVER_FETCH_ENABLED=true.")
			}
			meta["fetch"] = true
			params.Status = "pending"
		}
		if in.Title == "" {
			params.Title = u
		}
	case "captured_page":
		if in.CapturedPageID == nil {
			return errField("captured_page_id", "is required")
		}
		if err := s.can(r, auth.PermProductsRead); err != nil {
			return err
		}
		cp, err := s.db.GetCapturedPage(r.Context(), *in.CapturedPageID)
		if err != nil {
			return errField("captured_page_id", "does not refer to an existing capture")
		}
		params.CapturedPageID = &cp.ID
		params.Url = &cp.Url
		if in.Title == "" {
			params.Title = firstNonEmptyStr(cp.Title, cp.Url)
		}
		if cp.HtmlSnapshot != nil {
			params.ContentText = *cp.HtmlSnapshot
			params.MimeType = "text/html"
		}
		var sels, extracted, structured any
		_ = json.Unmarshal(cp.Selections, &sels)
		_ = json.Unmarshal(cp.Extracted, &extracted)
		_ = json.Unmarshal(cp.StructuredData, &structured)
		meta["selections"], meta["extracted"], meta["structured_data"] = sels, extracted, structured
	case "html":
		if strings.TrimSpace(in.Content) == "" {
			return errField("content", "is required")
		}
		params.ContentText, params.MimeType = in.Content, "text/html"
		if in.URL != nil && normalize.ValidHTTPURL(*in.URL) {
			params.Url = in.URL
		}
	case "repository_path":
		real, err := s.resolveRepoPath(in.Path)
		if err != nil {
			return err
		}
		meta["path"] = real
		if in.Title == "" {
			params.Title = filepath.Base(real)
		}
	case "openapi":
		if _, err := analysis.ParseStructured(in.Content); err != nil {
			return errField("content", err.Error())
		}
		params.ContentText, params.MimeType = in.Content, "application/yaml"
	case "api_examples":
		clean, err := sanitizeExamples(in.Content)
		if err != nil {
			return err
		}
		params.ContentText, params.MimeType = clean, "application/json"
	case "text":
		if strings.TrimSpace(in.Content) == "" {
			return errField("content", "is required")
		}
		params.ContentText, params.MimeType = normalize.MultilineText(in.Content), "text/plain"
	case "workflow":
		v := valid.New()
		v.Check(len(in.Steps) >= 1 && len(in.Steps) <= 200, "steps", "must contain between 1 and 200 steps")
		var lines []string
		for i := range in.Steps {
			st := &in.Steps[i]
			st.Actor, st.Action, st.System = normalize.Text(st.Actor), normalize.Text(st.Action), normalize.Text(st.System)
			st.Outcome, st.Decision = normalize.Text(st.Outcome), normalize.Text(st.Decision)
			v.Check(st.Action != "", "steps", fmt.Sprintf("step %d needs an action", i+1))
			v.Check(len(st.Action) <= 500 && len(st.Actor) <= 120 && len(st.System) <= 120 && len(st.Outcome) <= 500, "steps", "step text is too long")
			lines = append(lines, fmt.Sprintf("%d. %s: %s -> %s", i+1, st.Actor, st.Action, st.Outcome))
		}
		if err := checkValid(v); err != nil {
			return err
		}
		meta["steps"] = in.Steps
		params.ContentText, params.MimeType = strings.Join(lines, "\n"), "text/plain"
	}
	if params.Title == "" {
		params.Title = in.Title
	}
	if params.Title == "" {
		params.Title = strings.ReplaceAll(in.Kind, "_", " ") + " source"
	}
	params.Title = normalize.Truncate(params.Title, 300)
	params.Metadata = jsonOf(meta)
	params.SizeBytes = int64(len(params.ContentText))
	if params.ContentText != "" {
		params.ContentSha256 = sha256Hex(params.ContentText)
	}
	src, run, err := s.storeSource(r, project, params, in.AutoAnalyze == nil || *in.AutoAnalyze)
	if err != nil {
		return err
	}
	return created(w, map[string]any{"source": sourceSummary(src), "run": run})
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// storeSource saves a source and optionally queues its default analyzer.
func (s *Server) storeSource(r *http.Request, project store.AnalysisProject, params store.CreateAnalysisSourceParams, autoAnalyze bool) (store.AnalysisSource, *store.AnalysisRun, error) {
	var src store.AnalysisSource
	var run *store.AnalysisRun
	err := s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		var err error
		if src, err = q.CreateAnalysisSource(r.Context(), params); err != nil {
			return err
		}
		if params.AttachmentID != nil {
			et := "analysis_source"
			if err := q.LinkAttachment(r.Context(), store.LinkAttachmentParams{ID: *params.AttachmentID, EntityType: &et, EntityID: &src.ID}); err != nil {
				return err
			}
		}
		if autoAnalyze && src.Status != "failed" {
			if name := analysis.DefaultAnalyzer(src.Kind); name != "" {
				rn, err := q.EnqueueAnalysisRun(r.Context(), store.EnqueueAnalysisRunParams{ProjectID: project.ID, SourceID: &src.ID, Analyzer: name,
					Options: json.RawMessage("{}"), CreatedBy: userIDPtr(r)})
				if err != nil {
					return err
				}
				run = &rn
			}
		}
		if err := q.TouchAnalysisProject(r.Context(), project.ID); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "analysis.source_add", EntityType: "analysis_project", EntityID: &project.ID,
			Summary: "Added " + strings.ReplaceAll(src.Kind, "_", " ") + " source " + src.Title, Extra: map[string]any{"source_id": src.ID}})
	})
	if err == nil && run != nil {
		s.deps.Wake()
	}
	return src, run, err
}

func sourceSummary(src store.AnalysisSource) map[string]any {
	m := toMap(src)
	delete(m, "content_text")
	m["content_length"] = len(src.ContentText)
	return m
}

var uploadKinds = map[string]bool{"repository_archive": true, "document": true, "screenshot": true, "openapi": true, "api_examples": true, "html": true}

func (s *Server) handleUploadSource(w http.ResponseWriter, r *http.Request) error {
	project, err := s.projectFor(r)
	if err != nil {
		return err
	}
	att, sniffed, fields, err := s.saveUpload(r, nil, nil)
	if err != nil {
		return err
	}
	fail := func(e error) error {
		_ = s.cleanupAttachment(r, att)
		return e
	}
	kind := fields["kind"]
	if !uploadKinds[kind] {
		return fail(errField("kind", "must be repository_archive, document, screenshot, openapi, api_examples or html"))
	}
	title := normalize.Truncate(normalize.Text(firstNonEmptyStr(fields["title"], att.Filename)), 300)
	params := store.CreateAnalysisSourceParams{ProjectID: project.ID, Kind: kind, Title: title, AttachmentID: &att.ID,
		ContentSha256: att.Sha256, MimeType: att.ContentType, SizeBytes: att.SizeBytes, Metadata: jsonOf(map[string]any{"filename": att.Filename}),
		Status: "ready", CreatedBy: userIDPtr(r)}
	readAll := func() ([]byte, error) {
		f, err := s.files.Open(att.StorageKey)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		buf := make([]byte, att.SizeBytes)
		_, err = io.ReadFull(f, buf)
		return buf, err
	}
	switch kind {
	case "repository_archive":
		if !(strings.HasPrefix(sniffed, "application/zip") || strings.HasPrefix(sniffed, "application/x-gzip") || strings.HasPrefix(sniffed, "application/gzip")) {
			return fail(errField("file", "must be a .zip or .tar.gz archive"))
		}
	case "screenshot":
		if !strings.HasPrefix(sniffed, "image/") {
			return fail(errField("file", "must be a PNG, JPEG, GIF or WebP image"))
		}
	case "document":
		data, err := readAll()
		if err != nil {
			return fail(err)
		}
		text, err := analysis.ExtractText(att.Filename, data, maxSourceText)
		if err != nil {
			params.Status, params.Error = "failed", err.Error()
		}
		params.ContentText = text
	case "openapi", "api_examples", "html":
		data, err := readAll()
		if err != nil {
			return fail(err)
		}
		if len(data) > maxSourceText {
			return fail(errField("file", "is larger than 5 MB"))
		}
		text := string(data)
		switch kind {
		case "openapi":
			if _, err := analysis.ParseStructured(text); err != nil {
				return fail(errField("file", err.Error()))
			}
		case "api_examples":
			clean, err := sanitizeExamples(text)
			if err != nil {
				return fail(err)
			}
			text = clean
			// The original upload may contain credentials; keep only the sanitized text.
			params.AttachmentID = nil
			defer func() { _ = s.cleanupAttachment(r, att) }()
		}
		params.ContentText = text
	}
	src, run, err := s.storeSource(r, project, params, fields["auto_analyze"] != "false")
	if err != nil {
		return fail(err)
	}
	return created(w, map[string]any{"source": sourceSummary(src), "run": run})
}

func (s *Server) handleListSources(w http.ResponseWriter, r *http.Request) error {
	project, err := s.projectFor(r)
	if err != nil {
		return err
	}
	rows, err := s.db.ListAnalysisSources(r.Context(), project.ID)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": rows})
}

func (s *Server) handleGetSource(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "sourceID")
	if err != nil {
		return err
	}
	src, err := s.db.GetAnalysisSource(r.Context(), id)
	if err != nil {
		return err
	}
	m := sourceSummary(src)
	const preview = 200_000
	text := src.ContentText
	if len(text) > preview {
		text = text[:preview]
		m["content_truncated"] = true
	}
	m["content_text"] = text
	return ok(w, m)
}

func (s *Server) handleDeleteSource(w http.ResponseWriter, r *http.Request) error {
	project, err := s.projectFor(r)
	if err != nil {
		return err
	}
	sid, err := pathUUID(r, "sourceID")
	if err != nil {
		return err
	}
	src, err := s.db.GetAnalysisSource(r.Context(), sid)
	if err != nil || src.ProjectID != project.ID {
		return errNotFound("Source")
	}
	if _, err := s.db.DeleteAnalysisSource(r.Context(), store.DeleteAnalysisSourceParams{ID: sid, ProjectID: project.ID}); err != nil {
		return err
	}
	if src.AttachmentID != nil {
		if a, err := s.db.GetAttachment(r.Context(), *src.AttachmentID); err == nil {
			_ = s.cleanupAttachment(r, a)
		}
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "analysis.source_delete", EntityType: "analysis_project", EntityID: &project.ID, Summary: "Deleted source " + src.Title})
	return noContent(w)
}

// ---- runs ----

type runInput struct {
	SourceID uuid.UUID `json:"source_id"`
	Analyzer string    `json:"analyzer"`
}

func (s *Server) handleCreateRun(w http.ResponseWriter, r *http.Request) error {
	project, err := s.projectFor(r)
	if err != nil {
		return err
	}
	var in runInput
	if err := decode(r, &in); err != nil {
		return err
	}
	src, err := s.db.GetAnalysisSource(r.Context(), in.SourceID)
	if err != nil || src.ProjectID != project.ID {
		return errField("source_id", "must be a source of this project")
	}
	if in.Analyzer == "" {
		in.Analyzer = analysis.DefaultAnalyzer(src.Kind)
	}
	if _, ok := analysis.Get(in.Analyzer); !ok {
		return errField("analyzer", "is not a known analyzer")
	}
	run, err := s.db.EnqueueAnalysisRun(r.Context(), store.EnqueueAnalysisRunParams{ProjectID: project.ID, SourceID: &src.ID, Analyzer: in.Analyzer,
		Options: json.RawMessage("{}"), CreatedBy: userIDPtr(r)})
	if err != nil {
		return err
	}
	s.deps.Wake()
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "analysis.run_queued", EntityType: "analysis_project", EntityID: &project.ID,
		Summary: "Queued " + in.Analyzer + " analysis of " + src.Title})
	writeJSON(w, http.StatusAccepted, run)
	return nil
}

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) error {
	project, err := s.projectFor(r)
	if err != nil {
		return err
	}
	rows, err := s.db.ListAnalysisRuns(r.Context(), project.ID)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": rows})
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "runID")
	if err != nil {
		return err
	}
	run, err := s.db.GetAnalysisRun(r.Context(), id)
	if err != nil {
		return err
	}
	return ok(w, run)
}

func (s *Server) handleCancelRun(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "runID")
	if err != nil {
		return err
	}
	n, err := s.db.CancelAnalysisRun(r.Context(), id)
	if err != nil {
		return err
	}
	if n == 0 {
		return errConflict("not_cancellable", "Only queued runs can be cancelled.")
	}
	return noContent(w)
}

// ---- findings ----

func (s *Server) handleListFindings(w http.ResponseWriter, r *http.Request) error {
	project, err := s.projectFor(r)
	if err != nil {
		return err
	}
	limit, offset := pagination(r)
	src, err := qUUID(r, "source_id")
	if err != nil {
		return err
	}
	rows, err := s.db.ListFindings(r.Context(), store.ListFindingsParams{ProjectID: project.ID, Category: qText(r, "category"),
		Classification: qText(r, "classification"), SourceID: src, Origin: qText(r, "origin"), Q: qText(r, "q"), Lim: limit, Off: offset})
	if err != nil {
		return err
	}
	out := make([]FindingListItem, 0, len(rows))
	var total int64
	for _, row := range rows {
		out = append(out, FindingListItem{row.AnalysisFinding, row.SourceTitle, row.SourceKind})
		total = row.Total
	}
	return ok(w, Page[FindingListItem]{Items: out, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleFindingSummary(w http.ResponseWriter, r *http.Request) error {
	project, err := s.projectFor(r)
	if err != nil {
		return err
	}
	rows, err := s.db.FindingCategoryCounts(r.Context(), project.ID)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": rows})
}

type findingInput struct {
	SourceID        *uuid.UUID      `json:"source_id"`
	Category        string          `json:"category"`
	Title           string          `json:"title"`
	Detail          string          `json:"detail"`
	Evidence        string          `json:"evidence"`
	EvidenceLocator string          `json:"evidence_locator"`
	Classification  string          `json:"classification"`
	Confidence      decimal.Decimal `json:"confidence"`
	Assumptions     string          `json:"assumptions"`
}

var categoryRe = regexp.MustCompile(`^[a-z][a-z_]{1,40}$`)

func (in *findingInput) validate() error {
	in.Title, in.Category = normalize.Text(in.Title), strings.ToLower(normalize.Text(in.Category))
	in.Detail, in.Evidence = normalize.MultilineText(in.Detail), normalize.MultilineText(in.Evidence)
	in.EvidenceLocator, in.Assumptions = normalize.Text(in.EvidenceLocator), normalize.MultilineText(in.Assumptions)
	v := valid.New()
	v.Check(categoryRe.MatchString(in.Category), "category", "must be 2-41 lowercase letters or underscores")
	v.Len("title", in.Title, 1, 500)
	v.Max("detail", in.Detail, 20000)
	v.Max("evidence", in.Evidence, 8000)
	v.Max("evidence_locator", in.EvidenceLocator, 1000)
	v.OneOf("classification", in.Classification, classifications...)
	v.Check(!in.Confidence.IsNegative() && in.Confidence.LessThanOrEqual(decimal.NewFromInt(1)), "confidence", "must be between 0 and 1")
	if in.Classification == "inferred" {
		v.Check(in.Assumptions != "", "assumptions", "are required for inferred findings")
	}
	if in.Classification == "verified" {
		v.Check(in.Evidence != "", "evidence", "is required for verified findings")
	}
	return checkValid(v)
}

func (s *Server) handleCreateFinding(w http.ResponseWriter, r *http.Request) error {
	project, err := s.projectFor(r)
	if err != nil {
		return err
	}
	var in findingInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(); err != nil {
		return err
	}
	if in.SourceID != nil {
		src, err := s.db.GetAnalysisSource(r.Context(), *in.SourceID)
		if err != nil || src.ProjectID != project.ID {
			return errField("source_id", "must be a source of this project")
		}
	}
	f, err := s.db.CreateFinding(r.Context(), store.CreateFindingParams{ProjectID: project.ID, SourceID: in.SourceID, Category: in.Category,
		Title: in.Title, Detail: in.Detail, Evidence: in.Evidence, EvidenceLocator: in.EvidenceLocator, Classification: in.Classification,
		Confidence: in.Confidence.Round(3), Assumptions: in.Assumptions, Origin: "manual", Data: json.RawMessage("{}"), CreatedBy: userIDPtr(r)})
	if err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "analysis.finding_create", EntityType: "analysis_project", EntityID: &project.ID, Summary: "Added finding " + f.Title})
	return created(w, f)
}

func (s *Server) handleGetFinding(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "findingID")
	if err != nil {
		return err
	}
	f, err := s.db.GetFinding(r.Context(), id)
	if err != nil {
		return err
	}
	return ok(w, f)
}

func (s *Server) handleUpdateFinding(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "findingID")
	if err != nil {
		return err
	}
	var in findingInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(); err != nil {
		return err
	}
	before, err := s.db.GetFinding(r.Context(), id)
	if err != nil {
		return err
	}
	f, err := s.db.UpdateFinding(r.Context(), store.UpdateFindingParams{ID: id, Category: in.Category, Title: in.Title, Detail: in.Detail,
		Evidence: in.Evidence, EvidenceLocator: in.EvidenceLocator, Classification: in.Classification, Confidence: in.Confidence.Round(3),
		Assumptions: in.Assumptions})
	if err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "analysis.finding_update", EntityType: "analysis_project", EntityID: &f.ProjectID,
		Summary: "Edited finding " + f.Title, Before: before, After: f})
	return ok(w, f)
}

func (s *Server) handleDeleteFinding(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "findingID")
	if err != nil {
		return err
	}
	f, err := s.db.GetFinding(r.Context(), id)
	if err != nil {
		return err
	}
	if _, err := s.db.DeleteFinding(r.Context(), store.DeleteFindingParams{ID: id, ProjectID: f.ProjectID}); err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "analysis.finding_delete", EntityType: "analysis_project", EntityID: &f.ProjectID, Summary: "Deleted finding " + f.Title})
	return noContent(w)
}

// ---- reports ----

// reportInputFor collects the project material passed to the report generator.
func (s *Server) reportInputFor(r *http.Request, project store.AnalysisProject) (analysis.ReportInput, error) {
	srcs, err := s.db.ListAnalysisSources(r.Context(), project.ID)
	if err != nil {
		return analysis.ReportInput{}, err
	}
	fs, err := s.db.AllProjectFindings(r.Context(), project.ID)
	if err != nil {
		return analysis.ReportInput{}, err
	}
	in := analysis.ReportInput{ProjectName: project.Name, Objective: project.Objective, Description: project.Description,
		AuthorizationBasis: project.AuthorizationBasis, GeneratedAt: time.Now()}
	for _, src := range srcs {
		in.Sources = append(in.Sources, analysis.ReportSource{ID: src.ID.String(), Title: src.Title, Kind: src.Kind})
	}
	for _, f := range fs {
		var data map[string]any
		_ = json.Unmarshal(f.Data, &data)
		conf, _ := f.Confidence.Float64()
		rf := analysis.ReportFinding{ID: f.ID.String(), Category: f.Category, Title: f.Title, Detail: f.Detail, Evidence: f.Evidence,
			Locator: f.EvidenceLocator, Classification: f.Classification, Confidence: conf, Assumptions: f.Assumptions, Origin: f.Origin,
			Data: data, SourceTitle: deref(f.SourceTitle)}
		if f.SourceID != nil {
			rf.SourceID = f.SourceID.String()
		}
		in.Findings = append(in.Findings, rf)
	}
	return in, nil
}

type reportInput struct {
	Kind            string `json:"kind"`
	Title           string `json:"title"`
	ContentMarkdown string `json:"content_markdown"`
}

func (s *Server) handleCreateReport(w http.ResponseWriter, r *http.Request) error {
	project, err := s.projectFor(r)
	if err != nil {
		return err
	}
	var in reportInput
	if err := decodeLimit(r, &in, 3<<20); err != nil {
		return err
	}
	params := store.CreateReportParams{ProjectID: project.ID, Kind: in.Kind, FindingIds: []uuid.UUID{}, SourceIds: []uuid.UUID{}, CreatedBy: userIDPtr(r)}
	if strings.TrimSpace(in.ContentMarkdown) != "" {
		in.Title = normalize.Text(in.Title)
		v := valid.New()
		v.OneOf("kind", in.Kind, append(append([]string{}, analysis.ReportKinds...), "custom", "comparison")...)
		v.Len("title", in.Title, 1, 300)
		v.Max("content_markdown", in.ContentMarkdown, 2_000_000)
		if err := checkValid(v); err != nil {
			return err
		}
		params.Title, params.ContentMarkdown, params.Origin = in.Title, in.ContentMarkdown, "manual"
	} else {
		v := valid.New()
		v.OneOf("kind", in.Kind, analysis.ReportKinds...)
		if err := checkValid(v); err != nil {
			return err
		}
		ri, err := s.reportInputFor(r, project)
		if err != nil {
			return err
		}
		title, md, used, err := analysis.GenerateReport(in.Kind, ri)
		if err != nil {
			return errBadRequest(err.Error())
		}
		if t := normalize.Text(in.Title); t != "" {
			title = t
		}
		params.Title, params.ContentMarkdown, params.Origin = normalize.Truncate(title, 300), md, "rule"
		for _, id := range used {
			if u, err := uuid.Parse(id); err == nil {
				params.FindingIds = append(params.FindingIds, u)
			}
		}
		for _, src := range ri.Sources {
			if u, err := uuid.Parse(src.ID); err == nil {
				params.SourceIds = append(params.SourceIds, u)
			}
		}
	}
	rep, err := s.db.CreateReport(r.Context(), params)
	if err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "analysis.report_create", EntityType: "analysis_project", EntityID: &project.ID,
		Summary: "Created report " + rep.Title, Extra: map[string]any{"report_id": rep.ID, "origin": rep.Origin}})
	return created(w, rep)
}

func (s *Server) handleListReports(w http.ResponseWriter, r *http.Request) error {
	project, err := s.projectFor(r)
	if err != nil {
		return err
	}
	rows, err := s.db.ListReports(r.Context(), project.ID)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": rows})
}

func (s *Server) handleGetReport(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "reportID")
	if err != nil {
		return err
	}
	rep, err := s.db.GetReport(r.Context(), id)
	if err != nil {
		return err
	}
	cited, err := s.db.FindingsByIDs(r.Context(), store.FindingsByIDsParams{ProjectID: rep.ProjectID, Ids: rep.FindingIds})
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"report": rep, "findings": cited})
}

func (s *Server) handleExportReport(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "reportID")
	if err != nil {
		return err
	}
	rep, err := s.db.GetReport(r.Context(), id)
	if err != nil {
		return err
	}
	slug := normalize.Slug(rep.Title)
	switch r.URL.Query().Get("format") {
	case "", "md", "markdown":
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+slug+`.md"`)
		body := rep.ContentMarkdown
		if rep.IsAiGenerated && !strings.Contains(body, "AI-generated") {
			body = "> AI-generated content (" + rep.ProviderName + " / " + rep.Model + "). Verify against the cited evidence.\n\n" + body
		}
		_, _ = w.Write([]byte(body))
		return nil
	case "json":
		cited, err := s.db.FindingsByIDs(r.Context(), store.FindingsByIDsParams{ProjectID: rep.ProjectID, Ids: rep.FindingIds})
		if err != nil {
			return err
		}
		w.Header().Set("Content-Disposition", `attachment; filename="`+slug+`.json"`)
		return ok(w, map[string]any{"report": rep, "findings": cited})
	}
	return errField("format", "must be md or json")
}

func (s *Server) handleDeleteReport(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "reportID")
	if err != nil {
		return err
	}
	rep, err := s.db.GetReport(r.Context(), id)
	if err != nil {
		return err
	}
	if _, err := s.db.DeleteReport(r.Context(), store.DeleteReportParams{ID: id, ProjectID: rep.ProjectID}); err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "analysis.report_delete", EntityType: "analysis_project", EntityID: &rep.ProjectID, Summary: "Deleted report " + rep.Title})
	return noContent(w)
}
