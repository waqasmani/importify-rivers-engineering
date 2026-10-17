package httpapi

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/auth"
)

func (s *Server) mountAdmin(r chi.Router) {
	r.With(s.requireSession).Get("/auth/me/export", s.h(s.handleExportMyData))
	r.Group(func(r chi.Router) {
		r.Use(s.requireSession)
		r.With(s.requirePerm(auth.PermDataExport)).Get("/admin/export", s.h(s.handleFullExport))
		r.With(s.requirePerm(auth.PermBackupsManage)).Get("/admin/backups", s.h(s.handleListBackups))
		r.With(s.requirePerm(auth.PermBackupsManage)).Post("/admin/backups", s.h(s.handleCreateBackup))
		r.With(s.requirePerm(auth.PermBackupsManage)).Get("/admin/backups/{name}", s.h(s.handleDownloadBackup))
		r.With(s.requirePerm(auth.PermBackupsManage)).Delete("/admin/backups/{name}", s.h(s.handleDeleteBackup))
	})
}

// exportTables lists what a full export contains. Credentials, sessions, password hashes and
// encrypted provider keys are never exported.
var exportTables = []struct{ name, query string }{
	{"users", "SELECT id, email, display_name, role_id, is_owner, is_active, last_login_at, created_at, updated_at FROM users"},
	{"roles", "SELECT * FROM roles"},
	{"role_permissions", "SELECT * FROM role_permissions"},
	{"workspaces", "SELECT * FROM workspaces"},
	{"app_settings", "SELECT key, value, updated_at FROM app_settings"},
	{"pipelines", "SELECT * FROM pipelines"},
	{"pipeline_stages", "SELECT * FROM pipeline_stages"},
	{"companies", "SELECT * FROM companies"},
	{"contacts", "SELECT * FROM contacts"},
	{"suppliers", "SELECT * FROM suppliers"},
	{"leads", "SELECT * FROM leads"},
	{"opportunities", "SELECT * FROM opportunities"},
	{"opportunity_products", "SELECT * FROM opportunity_products"},
	{"tasks", "SELECT * FROM tasks"},
	{"notes", "SELECT * FROM notes"},
	{"attachments", "SELECT id, entity_type, entity_id, filename, content_type, size_bytes, sha256, uploaded_by, created_at FROM attachments"},
	{"tags", "SELECT * FROM tags"},
	{"record_tags", "SELECT * FROM record_tags"},
	{"custom_field_definitions", "SELECT * FROM custom_field_definitions"},
	{"source_adapters", "SELECT * FROM source_adapters"},
	{"captured_pages", "SELECT * FROM captured_pages"},
	{"products", "SELECT * FROM products"},
	{"product_variants", "SELECT * FROM product_variants"},
	{"product_images", "SELECT * FROM product_images"},
	{"supplier_products", "SELECT * FROM supplier_products"},
	{"product_imports", "SELECT * FROM product_imports"},
	{"analysis_projects", "SELECT * FROM analysis_projects"},
	{"analysis_sources", "SELECT * FROM analysis_sources"},
	{"analysis_runs", "SELECT * FROM analysis_runs"},
	{"analysis_findings", "SELECT * FROM analysis_findings"},
	{"analysis_reports", "SELECT * FROM analysis_reports"},
	{"ai_provider_configs", "SELECT id, name, kind, base_url, model, is_default, enabled, is_local, allow_external_data, supports_vision, temperature, max_output_tokens, timeout_seconds, created_at, updated_at FROM ai_provider_configs"},
	{"ai_jobs", "SELECT * FROM ai_jobs"},
	{"extension_devices", "SELECT id, user_id, name, browser, extension_version, created_at, last_seen_at, revoked_at FROM extension_devices"},
	{"audit_logs", "SELECT * FROM audit_logs ORDER BY id DESC LIMIT 200000"},
}

// writeJSONLines streams query rows as JSON Lines into w.
func (s *Server) writeJSONLines(ctx context.Context, w io.Writer, query string, args ...any) (int, error) {
	rows, err := s.db.Pool.Query(ctx, "SELECT row_to_json(t)::text FROM ("+query+") t", args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return n, err
		}
		if _, err := io.WriteString(w, line+"\n"); err != nil {
			return n, err
		}
		n++
	}
	return n, rows.Err()
}

func (s *Server) handleFullExport(w http.ResponseWriter, r *http.Request) error {
	stamp := time.Now().UTC().Format("20060102-150405")
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="rea-export-`+stamp+`.zip"`)
	w.Header().Set("Cache-Control", "no-store")
	zw := zip.NewWriter(w)
	counts := map[string]int{}
	for _, t := range exportTables {
		f, err := zw.Create(t.name + ".jsonl")
		if err != nil {
			return err
		}
		n, err := s.writeJSONLines(r.Context(), f, t.query)
		if err != nil {
			s.log.Error("export table failed", "table", t.name, "error", err.Error())
			break
		}
		counts[t.name] = n
	}
	manifest, _ := json.MarshalIndent(map[string]any{"format": "rea-export-v1", "exported_at": time.Now().UTC(), "server_version": s.cfg.Version,
		"tables": counts, "notes": "JSON Lines, one row per line. Passwords, sessions, API tokens and encrypted AI keys are excluded. Attachment files are in the data directory backup."}, "", "  ")
	if f, err := zw.Create("manifest.json"); err == nil {
		_, _ = f.Write(manifest)
	}
	if err := zw.Close(); err != nil {
		return nil
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "data.full_export", Summary: "Downloaded a full data export", Extra: map[string]any{"tables": counts}})
	return nil
}

// handleExportMyData gives a signed-in user everything REA stores about them.
func (s *Server) handleExportMyData(w http.ResponseWriter, r *http.Request) error {
	uid := principal(r).User.ID
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="rea-my-data.zip"`)
	zw := zip.NewWriter(w)
	parts := []struct{ name, query string }{
		{"profile", "SELECT id, email, display_name, role_id, is_owner, is_active, last_login_at, created_at FROM users WHERE id = $1"},
		{"sessions", "SELECT id, ip_address, user_agent, created_at, last_seen_at, expires_at, revoked_at FROM user_sessions WHERE user_id = $1"},
		{"extension_devices", "SELECT id, name, browser, extension_version, created_at, last_seen_at, revoked_at FROM extension_devices WHERE user_id = $1"},
		{"captured_pages", "SELECT id, url, title, page_type, captured_at FROM captured_pages WHERE captured_by = $1"},
		{"notes", "SELECT * FROM notes WHERE author_id = $1"},
		{"tasks_assigned", "SELECT * FROM tasks WHERE assignee_id = $1"},
		{"ai_jobs", "SELECT id, tool, status, provider_name, model, entity_type, entity_id, queued_at, finished_at FROM ai_jobs WHERE requested_by = $1"},
		{"audit_logs", "SELECT * FROM audit_logs WHERE actor_user_id = $1 ORDER BY id DESC LIMIT 50000"},
	}
	for _, p := range parts {
		f, err := zw.Create(p.name + ".jsonl")
		if err != nil {
			return err
		}
		if _, err := s.writeJSONLines(r.Context(), f, p.query, uid); err != nil {
			break
		}
	}
	_ = zw.Close()
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "data.personal_export", EntityType: "user", EntityID: &uid, Summary: "Downloaded own data"})
	return nil
}

// ---- database backups ----

var backupNameRe = regexp.MustCompile(`^rea-\d{8}-\d{6}\.dump$`)

func (s *Server) backupDir() string { return filepath.Join(s.cfg.DataDir, "backups") }

type backupInfo struct {
	Name      string    `json:"name"`
	SizeBytes int64     `json:"size_bytes"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Server) handleListBackups(w http.ResponseWriter, r *http.Request) error {
	entries, err := os.ReadDir(s.backupDir())
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	out := []backupInfo{}
	for _, e := range entries {
		if !backupNameRe.MatchString(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, backupInfo{e.Name(), info.Size(), info.ModTime()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	_, lookErr := exec.LookPath(s.cfg.PgDumpPath)
	return ok(w, map[string]any{"items": out, "pg_dump_available": lookErr == nil, "directory": s.backupDir(),
		"restore_hint": "Restore with deploy/scripts/restore.sh; restoring replaces the current database."})
}

// pgEnv converts the database URL into libpq environment variables so the password never appears
// in a process argument list.
func pgEnv(dbURL string) ([]string, error) {
	u, err := url.Parse(dbURL)
	if err != nil {
		return nil, err
	}
	env := []string{"PGHOST=" + u.Hostname(), "PGDATABASE=" + strings.TrimPrefix(u.Path, "/")}
	if p := u.Port(); p != "" {
		env = append(env, "PGPORT="+p)
	}
	if u.User != nil {
		env = append(env, "PGUSER="+u.User.Username())
		if pw, ok := u.User.Password(); ok {
			env = append(env, "PGPASSWORD="+pw)
		}
	}
	if m := u.Query().Get("sslmode"); m != "" {
		env = append(env, "PGSSLMODE="+m)
	}
	return env, nil
}

func (s *Server) handleCreateBackup(w http.ResponseWriter, r *http.Request) error {
	bin, err := exec.LookPath(s.cfg.PgDumpPath)
	if err != nil {
		return errUnavailable("pg_dump_missing", "pg_dump is not installed where the API runs. Use deploy/scripts/backup.sh on the host instead.")
	}
	if err := os.MkdirAll(s.backupDir(), 0o750); err != nil {
		return err
	}
	name := "rea-" + time.Now().UTC().Format("20060102-150405") + ".dump"
	path := filepath.Join(s.backupDir(), name)
	env, err := pgEnv(s.cfg.DatabaseURL)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "--format=custom", "--no-owner", "--no-acl", "--file", path)
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.TempDir()}, env...)
	outBytes, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.Remove(path)
		msg := strings.TrimSpace(string(outBytes))
		if len(msg) > 400 {
			msg = msg[:400]
		}
		return &APIError{Status: http.StatusInternalServerError, Code: "backup_failed", Message: "pg_dump failed: " + msg}
	}
	_ = os.Chmod(path, 0o600)
	info, _ := os.Stat(path)
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "backup.create", Summary: "Created database backup " + name})
	return created(w, backupInfo{name, info.Size(), info.ModTime()})
}

func (s *Server) backupPath(r *http.Request) (string, string, error) {
	name := chi.URLParam(r, "name")
	if !backupNameRe.MatchString(name) {
		return "", "", errNotFound("Backup")
	}
	p := filepath.Join(s.backupDir(), name)
	if _, err := os.Stat(p); err != nil {
		return "", "", errNotFound("Backup")
	}
	return name, p, nil
}

func (s *Server) handleDownloadBackup(w http.ResponseWriter, r *http.Request) error {
	name, p, err := s.backupPath(r)
	if err != nil {
		return err
	}
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()
	st, _ := f.Stat()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "backup.download", Summary: "Downloaded database backup " + name})
	http.ServeContent(w, r, name, st.ModTime(), f)
	return nil
}

func (s *Server) handleDeleteBackup(w http.ResponseWriter, r *http.Request) error {
	name, p, err := s.backupPath(r)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "backup.delete", Summary: "Deleted database backup " + name})
	return noContent(w)
}
