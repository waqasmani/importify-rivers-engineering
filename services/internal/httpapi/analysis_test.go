package httpapi_test

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readFixture(t *testing.T, name string) string {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "fixtures", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func (e *env) workspaceID() string {
	return e.owner.Do("GET", "/api/v1/workspaces", nil).Must(e.t, 200).Items()[0]["id"].(string)
}

func (e *env) project() string {
	return e.owner.Do("POST", "/api/v1/analysis/projects", map[string]any{"workspace_id": e.workspaceID(), "name": "Storefront study",
		"objective": "Document a supplier storefront", "authorization_basis": "public_information", "authorization_confirmed": true,
		"analysis_types": []string{"website", "api"}}).Must(e.t, 201).Str("id")
}

func TestAnalysisRequiresAuthorization(t *testing.T) {
	e := newEnv(t)
	ws := e.workspaceID()
	res := e.owner.Do("POST", "/api/v1/analysis/projects", map[string]any{"workspace_id": ws, "name": "No consent",
		"authorization_basis": "public_information"}).Must(t, 422)
	if res.Get("error.fields.authorization_confirmed") == nil {
		t.Fatalf("authorization must be confirmed: %s", res.Body)
	}
	e.owner.Do("POST", "/api/v1/analysis/projects", map[string]any{"workspace_id": ws, "name": "Client", "authorization_basis": "client_authorized",
		"authorization_confirmed": true}).Must(t, 422)
	ok := e.owner.Do("POST", "/api/v1/analysis/projects", map[string]any{"workspace_id": ws, "name": "Client", "authorization_basis": "client_authorized",
		"authorization_note": "Signed statement of work with Acme Ltd, 2026-09", "authorization_confirmed": true}).Must(t, 201)
	if ok.Get("authorization_confirmed_at") == nil || ok.Get("authorization_confirmed_by") == nil {
		t.Fatalf("authorization not recorded: %s", ok.Body)
	}
	viewer := e.userWithRole("viewer", "v@example.com")
	viewer.Do("POST", "/api/v1/analysis/projects", map[string]any{"workspace_id": ws, "name": "x", "authorization_basis": "owned", "authorization_confirmed": true}).Must(t, 403)
}

func TestWebsiteAnalysisEndToEnd(t *testing.T) {
	e := newEnv(t)
	pid := e.project()
	src := e.owner.Do("POST", "/api/v1/analysis/projects/"+pid+"/sources", map[string]any{"kind": "html", "title": "Product page",
		"url": "https://shop.northwind.example/products/trailblazer-2l", "content": readFixture(t, "product-jsonld.html")}).Must(t, 201)
	if src.Str("run.status") != "queued" || src.Str("run.analyzer") != "website" {
		t.Fatalf("run not queued: %s", src.Body)
	}
	e.worker.Drain(t.Context())
	runs := e.owner.Do("GET", "/api/v1/analysis/projects/"+pid+"/runs", nil).Must(t, 200).Items()
	if runs[0]["status"] != "succeeded" {
		t.Fatalf("run did not succeed: %v", runs[0])
	}
	f := e.owner.Do("GET", "/api/v1/analysis/projects/"+pid+"/findings?category=product", nil).Must(t, 200)
	if f.Get("total").(float64) < 1 {
		t.Fatalf("no product findings: %s", f.Body)
	}
	unk := e.owner.Do("GET", "/api/v1/analysis/projects/"+pid+"/findings?classification=unknown", nil).Must(t, 200)
	if unk.Get("total").(float64) < 2 {
		t.Fatal("unknowns must be recorded")
	}
	// Re-running replaces rule findings instead of duplicating them.
	before := e.owner.Do("GET", "/api/v1/analysis/projects/"+pid+"/findings", nil).Get("total")
	e.owner.Do("POST", "/api/v1/analysis/projects/"+pid+"/runs", map[string]any{"source_id": src.Str("source.id")}).Must(t, 202)
	e.worker.Drain(t.Context())
	if after := e.owner.Do("GET", "/api/v1/analysis/projects/"+pid+"/findings", nil).Get("total"); after != before {
		t.Fatalf("re-run duplicated findings: %v -> %v", before, after)
	}

	// Manual findings are validated.
	e.owner.Do("POST", "/api/v1/analysis/projects/"+pid+"/findings", map[string]any{"category": "observation", "title": "Checkout uses a third-party widget",
		"classification": "inferred", "confidence": 0.6}).Must(t, 422)
	man := e.owner.Do("POST", "/api/v1/analysis/projects/"+pid+"/findings", map[string]any{"category": "observation", "title": "Checkout uses a third-party widget",
		"classification": "inferred", "confidence": 0.6, "assumptions": "Iframe origin differs from the shop domain"}).Must(t, 201)
	if man.Str("origin") != "manual" {
		t.Fatal("manual finding origin wrong")
	}

	for _, kind := range []string{"summary", "technical_documentation", "reconstruction_plan", "data_model", "architecture", "api_inventory"} {
		rep := e.owner.Do("POST", "/api/v1/analysis/projects/"+pid+"/reports", map[string]any{"kind": kind}).Must(t, 201)
		if rep.Str("origin") != "rule" || rep.Get("is_ai_generated") != false {
			t.Fatalf("%s: wrong origin: %s", kind, rep.Body)
		}
		if kind == "reconstruction_plan" {
			got := e.owner.Do("GET", "/api/v1/analysis/reports/"+rep.Str("id"), nil).Must(t, 200)
			if len(got.Get("findings").([]any)) == 0 {
				t.Fatal("report must reference findings")
			}
			md := e.owner.Do("GET", "/api/v1/analysis/reports/"+rep.Str("id")+"/export?format=md", nil).Must(t, 200)
			if !strings.Contains(string(md.Body), "## Evidence index") || !strings.Contains(md.Header.Get("Content-Disposition"), ".md") {
				t.Fatalf("markdown export wrong: %v", md.Header)
			}
		}
	}
	e.owner.Do("POST", "/api/v1/analysis/projects/"+pid+"/reports", map[string]any{"kind": "bogus"}).Must(t, 422)
	custom := e.owner.Do("POST", "/api/v1/analysis/projects/"+pid+"/reports", map[string]any{"kind": "custom", "title": "Interview notes",
		"content_markdown": "# Notes\nSupplier confirmed MOQ of 500."}).Must(t, 201)
	if custom.Str("origin") != "manual" {
		t.Fatal("custom report origin wrong")
	}
}

func TestCodebaseArchiveAnalysis(t *testing.T) {
	e := newEnv(t)
	pid := e.project()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range map[string]string{
		"app/go.mod":            "module example.com/app\n\nrequire github.com/go-chi/chi/v5 v5.2.0\n",
		"app/main.go":           "package main\nfunc main(){ r.Get(\"/widgets\", h); _ = os.Getenv(\"APP_SECRET\") }\n",
		"app/.env":              "APP_SECRET=supersecretvalue\n",
		"app/db/001_init.sql":   "CREATE TABLE widgets (id serial primary key, name text);\n",
	} {
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte(content))
	}
	_ = zw.Close()
	up := e.owner.Upload("/api/v1/analysis/projects/"+pid+"/sources/upload", "file", "app.zip", buf.Bytes(),
		map[string]string{"kind": "repository_archive", "title": "App source"}).Must(t, 201)
	e.worker.Drain(t.Context())
	runs := e.owner.Do("GET", "/api/v1/analysis/projects/"+pid+"/runs", nil).Must(t, 200).Items()
	if runs[0]["status"] != "succeeded" {
		t.Fatalf("codebase run failed: %v", runs[0])
	}
	all := e.owner.Do("GET", "/api/v1/analysis/projects/"+pid+"/findings?limit=200", nil).Must(t, 200)
	body := string(all.Body)
	for _, want := range []string{"GET /widgets", "Data model: widgets", "APP_SECRET", "secrets were skipped"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in findings", want)
		}
	}
	if strings.Contains(body, "supersecretvalue") {
		t.Fatal("secret value leaked into findings")
	}
	// A zip-slip archive is rejected by the worker with a clear error.
	var evil bytes.Buffer
	zw = zip.NewWriter(&evil)
	w, _ := zw.Create("../../etc/cron.d/evil")
	_, _ = w.Write([]byte("x"))
	_ = zw.Close()
	e.owner.Upload("/api/v1/analysis/projects/"+pid+"/sources/upload", "file", "evil.zip", evil.Bytes(), map[string]string{"kind": "repository_archive"}).Must(t, 201)
	e.worker.Drain(t.Context())
	runs = e.owner.Do("GET", "/api/v1/analysis/projects/"+pid+"/runs", nil).Must(t, 200).Items()
	if runs[0]["status"] != "failed" || !strings.Contains(runs[0]["error"].(string), "unsafe path") {
		t.Fatalf("zip slip not rejected: %v", runs[0])
	}
	e.owner.Upload("/api/v1/analysis/projects/"+pid+"/sources/upload", "file", "notes.txt", []byte("hello"), map[string]string{"kind": "repository_archive"}).Must(t, 422)
	_ = up
}

func TestAPISourcesAreSanitized(t *testing.T) {
	e := newEnv(t)
	pid := e.project()
	e.owner.Do("POST", "/api/v1/analysis/projects/"+pid+"/sources", map[string]any{"kind": "openapi", "content": "not: [valid"}).Must(t, 422)
	e.owner.Do("POST", "/api/v1/analysis/projects/"+pid+"/sources", map[string]any{"kind": "openapi", "title": "Inventory API",
		"content": readFixture(t, "openapi-inventory.yaml")}).Must(t, 201)
	ex := e.owner.Do("POST", "/api/v1/analysis/projects/"+pid+"/sources", map[string]any{"kind": "api_examples", "title": "Portal traffic",
		"content": readFixture(t, "api-examples.har.json")}).Must(t, 201)
	stored := e.owner.Do("GET", "/api/v1/analysis/sources/"+ex.Str("source.id"), nil).Must(t, 200)
	if strings.Contains(stored.Str("content_text"), "secret-token-never-stored") || strings.Contains(stored.Str("content_text"), "sid=abc") {
		t.Fatalf("credentials stored: %s", stored.Str("content_text"))
	}
	e.worker.Drain(t.Context())
	eps := e.owner.Do("GET", "/api/v1/analysis/projects/"+pid+"/findings?category=endpoint&limit=100", nil).Must(t, 200)
	if eps.Get("total").(float64) < 6 {
		t.Fatalf("expected endpoints from both sources: %s", eps.Body)
	}
	inv := e.owner.Do("POST", "/api/v1/analysis/projects/"+pid+"/reports", map[string]any{"kind": "api_inventory"}).Must(t, 201)
	if !strings.Contains(inv.Str("content_markdown"), "/items/{itemId}") || !strings.Contains(inv.Str("content_markdown"), "/api/orders/{id}") {
		t.Fatalf("api inventory incomplete: %s", inv.Str("content_markdown"))
	}
}

func TestWorkflowDocumentAndCaptureSources(t *testing.T) {
	e := newEnv(t)
	pid := e.project()
	e.owner.Do("POST", "/api/v1/analysis/projects/"+pid+"/sources", map[string]any{"kind": "workflow", "title": "Sample ordering",
		"steps": []map[string]any{{"actor": "Buyer", "action": "Request samples", "system": "Portal", "data": []string{"Sample request"}},
			{"actor": "Supplier", "action": "Ship samples", "outcome": "Tracking number sent"}}}).Must(t, 201)
	e.owner.Do("POST", "/api/v1/analysis/projects/"+pid+"/sources", map[string]any{"kind": "workflow", "steps": []any{}}).Must(t, 422)
	e.owner.Upload("/api/v1/analysis/projects/"+pid+"/sources/upload", "file", "requirements.md", []byte(readFixture(t, "requirements.md")),
		map[string]string{"kind": "document"}).Must(t, 201)
	ext, _ := e.pairExtension()
	cp := ext.Do("POST", "/api/v1/captures", map[string]any{"url": "https://shop.example.com/p/1", "title": "Captured product",
		"selections": []map[string]any{{"kind": "table", "selector": "table.specs", "rows": [][]string{{"Capacity", "2 L"}}}}}).Must(t, 201)
	ext.Do("POST", "/api/v1/analysis/projects/"+pid+"/sources", map[string]any{"kind": "captured_page", "captured_page_id": cp.Str("id")}).Must(t, 201)
	e.owner.Do("POST", "/api/v1/analysis/projects/"+pid+"/sources", map[string]any{"kind": "repository_path", "path": "/etc"}).Must(t, 409)
	e.owner.Do("POST", "/api/v1/analysis/projects/"+pid+"/sources", map[string]any{"kind": "url", "url": "https://example.com", "fetch": true}).Must(t, 409)
	e.worker.Drain(t.Context())
	for _, r := range e.owner.Do("GET", "/api/v1/analysis/projects/"+pid+"/runs", nil).Must(t, 200).Items() {
		if r["status"] != "succeeded" {
			t.Fatalf("run failed: %v", r)
		}
	}
	body := string(e.owner.Do("GET", "/api/v1/analysis/projects/"+pid+"/findings?limit=200", nil).Must(t, 200).Body)
	for _, want := range []string{"Step 1: Request samples", "Workflow diagram", "lock an account", "User-selected table"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	wf := e.owner.Do("POST", "/api/v1/analysis/projects/"+pid+"/reports", map[string]any{"kind": "workflow"}).Must(t, 201)
	if !strings.Contains(wf.Str("content_markdown"), "```mermaid") {
		t.Fatal("workflow report lacks a diagram")
	}
	// Deleting the project removes its data.
	e.owner.Do("DELETE", "/api/v1/analysis/projects/"+pid, nil).Must(t, 204)
	var n int
	_ = e.db.Pool.QueryRow(t.Context(), "SELECT count(*) FROM analysis_findings").Scan(&n)
	if n != 0 {
		t.Fatalf("findings remain after project deletion: %d", n)
	}
}
