package analysis

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "fixtures", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func find(fs []Finding, cat, titleContains string) *Finding {
	for i := range fs {
		if fs[i].Category == cat && strings.Contains(fs[i].Title, titleContains) {
			return &fs[i]
		}
	}
	return nil
}

func run(t *testing.T, name string, in Input) Result {
	t.Helper()
	a, ok := Get(name)
	if !ok {
		t.Fatalf("analyzer %s not registered", name)
	}
	res, err := a.Analyze(context.Background(), in)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	for _, f := range res.Findings {
		if f.Classification != Verified && f.Classification != Inferred && f.Classification != Unknown {
			t.Fatalf("invalid classification %q", f.Classification)
		}
		if f.Confidence < 0 || f.Confidence > 1 {
			t.Fatalf("invalid confidence %v", f.Confidence)
		}
	}
	return res
}

func TestWebsiteAnalyzerProductPage(t *testing.T) {
	res := run(t, "website", Input{Kind: "html", URL: "https://shop.northwind.example/products/trailblazer-2l", Text: fixture(t, "product-jsonld.html")})
	p := find(res.Findings, "product", "Trailblazer")
	if p == nil || p.Classification != Verified || p.Data["sku"] != "NW-TB-2L" || p.Data["price"] != "39.95" || p.Data["brand"] != "Northwind" {
		t.Fatalf("product finding wrong: %+v", p)
	}
	if find(res.Findings, "organization", "Northwind Outdoor GmbH") == nil {
		t.Error("organization not found")
	}
	wp := find(res.Findings, "technology", "WordPress detected")
	if wp == nil || wp.Classification != Inferred || wp.Assumptions == "" {
		t.Errorf("technology inference should be inferred with assumptions: %+v", wp)
	}
	if find(res.Findings, "technology", "Google Tag Manager") == nil {
		t.Error("GTM not detected")
	}
	nav := find(res.Findings, "navigation", "Primary navigation")
	if nav == nil || !strings.Contains(nav.Evidence, "Drinkware") {
		t.Errorf("navigation missing: %+v", nav)
	}
	form := find(res.Findings, "form", "add-to-cart")
	if form == nil || !strings.Contains(form.Evidence, "quantity: number (required)") || strings.Contains(form.Evidence, "product_id") {
		t.Errorf("form analysis wrong: %+v", form)
	}
	if find(res.Findings, "backend", "not observable") == nil || find(res.Findings, "data_model", "not observable") == nil {
		t.Error("unknowns must be recorded for a rendered page")
	}
	acc := find(res.Findings, "accessibility", "lack alternative text")
	if acc == nil || acc.Data["missing_alt"] != 1 {
		t.Errorf("alt text stats wrong: %+v", acc)
	}
}

func TestWebsiteAnalyzerLoginPageNeverReadsValues(t *testing.T) {
	res := run(t, "website", Input{Kind: "html", URL: "https://portal.acme.example/login", Text: fixture(t, "login-page.html")})
	for _, f := range res.Findings {
		if strings.Contains(f.Evidence+f.Detail+f.Title, "should-never-be-read") || strings.Contains(f.Evidence, "tok123") {
			t.Fatalf("form values leaked into finding %q", f.Title)
		}
	}
	if find(res.Findings, "authentication", "Sign-in form present") == nil || find(res.Findings, "authentication", "not observable") == nil {
		t.Error("authentication findings missing")
	}
	if find(res.Findings, "technology", "Next.js") == nil || find(res.Findings, "technology", "Stripe") == nil {
		t.Error("technology markers missing")
	}
}

func TestWebsiteAnalyzerMalformedAndEmpty(t *testing.T) {
	res := run(t, "website", Input{Kind: "html", Text: fixture(t, "malformed.html")})
	if find(res.Findings, "structured_data", "Invalid JSON-LD") == nil {
		t.Error("invalid JSON-LD should be reported")
	}
	empty := run(t, "website", Input{Kind: "url", URL: "https://x.example"})
	if len(empty.Findings) != 1 || empty.Findings[0].Classification != Unknown {
		t.Fatalf("empty source should yield one unknown: %+v", empty.Findings)
	}
}

func TestCodebaseAnalyzer(t *testing.T) {
	files := []File{
		{Path: "go.mod", Content: []byte("module example.com/shop\n\ngo 1.26\n\nrequire (\n\tgithub.com/go-chi/chi/v5 v5.2.0\n\tgorm.io/gorm v1.25.0\n\tgolang.org/x/text v0.20.0 // indirect\n)\n")},
		{Path: "cmd/api/main.go", Content: []byte("package main\nimport \"example.com/shop/internal/orders\"\nfunc main() {\n r.Get(\"/orders\", h)\n r.Post(\"/orders/{id}/ship\", h)\n _ = os.Getenv(\"DATABASE_URL\")\n}\n")},
		{Path: "internal/orders/model.go", Content: []byte("package orders\ntype Order struct {\n ID int `gorm:\"primaryKey\"`\n Total float64\n}\n")},
		{Path: "web/package.json", Content: []byte(`{"dependencies": {"react": "^19.0.0", "express": "^5.0.0"}, "devDependencies": {"vite": "^8.0.0"}, "scripts": {"build": "vite build"}}`)},
		{Path: "web/server.js", Content: []byte("const app = express();\napp.get('/api/health', ok)\nconst key = process.env.STRIPE_KEY\nimport x from '../internal/x'\n")},
		{Path: "db/schema.sql", Content: []byte("CREATE TABLE IF NOT EXISTS customers (id bigint);\ncreate table invoices (id int);\n")},
		{Path: "api/app.py", Content: []byte("@app.route('/legacy', methods=['GET', 'POST'])\ndef legacy(): pass\nclass Invoice(db.Model):\n  pass\n")},
		{Path: "Dockerfile", Content: []byte("FROM golang:1.27 AS build\nFROM alpine:3.22\nCMD [\"/app\"]\n")},
		{Path: "docker-compose.yml", Content: []byte("services:\n  api:\n    build: .\n  db:\n    image: postgres\n")},
		{Path: ".env", Skipped: "not needed for analysis"},
		{Path: "secrets/id_rsa", Skipped: "not needed for analysis"},
	}
	res := run(t, "codebase", Input{Kind: "repository_archive", Files: files})
	for _, want := range []string{"GET /orders", "POST /orders/{id}/ship", "GET /api/health"} {
		if find(res.Findings, "route", want) == nil {
			t.Errorf("route %q missing", want)
		}
	}
	for _, m := range []string{"Order", "customers", "invoices", "Invoice"} {
		if find(res.Findings, "data_model", "Data model: "+m) == nil {
			t.Errorf("model %q missing", m)
		}
	}
	if f := find(res.Findings, "configuration", "Environment variables"); f == nil || !strings.Contains(f.Evidence, "DATABASE_URL") || !strings.Contains(f.Evidence, "STRIPE_KEY") {
		t.Errorf("env vars missing: %+v", f)
	}
	if find(res.Findings, "technology", "chi is used") == nil || find(res.Findings, "technology", "React is used") == nil {
		t.Error("frameworks not inferred")
	}
	sec := find(res.Findings, "security", "secrets were skipped")
	if sec == nil || !strings.Contains(sec.Evidence, ".env") || !strings.Contains(sec.Evidence, "id_rsa") {
		t.Errorf("sensitive files not reported: %+v", sec)
	}
	if find(res.Findings, "infrastructure", "Docker Compose services (2 services)") == nil {
		t.Error("compose services not parsed")
	}
	arch := find(res.Findings, "architecture", "Module dependencies")
	if arch == nil || !strings.Contains(arch.Evidence, "cmd → internal") {
		t.Errorf("module graph wrong: %+v", arch)
	}
	if !WantFile("src/app.ts", 10) || WantFile("node_modules/x/index.js", 10) || WantFile("config/.env.production", 10) || WantFile("logo.png", 10) {
		t.Error("WantFile filter wrong")
	}
}

func TestOpenAPIAnalyzer(t *testing.T) {
	res := run(t, "openapi", Input{Kind: "openapi", Text: fixture(t, "openapi-inventory.yaml")})
	if res.Stats["operations"] != 4 {
		t.Fatalf("expected 4 operations, got %v", res.Stats["operations"])
	}
	get := find(res.Findings, "endpoint", "GET /items/{itemId}")
	if get == nil || get.Data["secured"] != true || !strings.Contains(get.Evidence, "itemId (path, required)") {
		t.Errorf("endpoint wrong: %+v", get)
	}
	if h := find(res.Findings, "endpoint", "GET /health"); h == nil || h.Data["secured"] != false {
		t.Errorf("security override not applied: %+v", h)
	}
	item := find(res.Findings, "schema", "Schema: Item")
	if item == nil || !strings.Contains(item.Evidence, "location: Location") || !strings.Contains(item.Evidence, "sku: string (required)") {
		t.Errorf("schema wrong: %+v", item)
	}
	if find(res.Findings, "auth", "bearerAuth") == nil {
		t.Error("security scheme missing")
	}
	if _, err := (openAPIAnalyzer{}).Analyze(context.Background(), Input{Text: "title: not a spec"}); err == nil {
		t.Error("non-spec YAML should fail")
	}
}

func TestAPIExamplesStripsSecrets(t *testing.T) {
	res := run(t, "api_examples", Input{Kind: "api_examples", Text: fixture(t, "api-examples.har.json")})
	get := find(res.Findings, "endpoint", "GET /api/orders/{id}")
	if get == nil || !strings.Contains(get.Evidence, "200") || !strings.Contains(get.Evidence, "404") {
		t.Fatalf("templated endpoint wrong: %+v", get)
	}
	for _, f := range res.Findings {
		blob := f.Evidence + f.Detail + mustJSON(f.Data)
		for _, secret := range []string{"secret-token-never-stored", "sid=abc", "buyer@example.com", "shipped"} {
			if strings.Contains(blob, secret) {
				t.Fatalf("recorded value %q leaked into %q", secret, f.Title)
			}
		}
	}
	if TemplatePath("/v1/users/3f1c2b0e-1d2c-4f6a-9b7e-2a1b3c4d5e6f/orders/77") != "/v1/users/{id}/orders/{id}" {
		t.Error("path templating wrong")
	}
}

func TestDocumentAnalyzer(t *testing.T) {
	text, err := ExtractText("requirements.md", []byte(fixture(t, "requirements.md")), 100000)
	if err != nil {
		t.Fatal(err)
	}
	res := run(t, "document", Input{Kind: "document", Title: "Requirements", Text: text})
	if find(res.Findings, "structure", "Document outline") == nil {
		t.Error("outline missing")
	}
	reqs := 0
	for _, f := range res.Findings {
		if f.Category == "requirement" {
			reqs++
		}
	}
	if reqs < 3 {
		t.Errorf("expected at least 3 requirements, got %d", reqs)
	}
	if find(res.Findings, "reference", "Referenced URLs") == nil {
		t.Error("URL references missing")
	}
	if _, err := ExtractText("blob.bin", []byte{0, 1, 2, 0}, 100); err == nil {
		t.Error("binary document should be rejected")
	}
	if _, err := ExtractText("bad.pdf", []byte("%PDF-1.4 garbage"), 100); err == nil {
		t.Error("corrupt PDF should fail gracefully")
	}
}

func TestWorkflowAnalyzerAndMermaid(t *testing.T) {
	meta := map[string]any{"steps": []any{
		map[string]any{"actor": "Buyer", "action": "Request a quotation", "system": "Supplier portal", "data": []any{"Quotation request"}},
		map[string]any{"actor": "Supplier", "action": "Send price list", "system": "Email", "data": []any{"Price list"}, "decision": "Price approved?"},
		map[string]any{"actor": "Buyer", "action": "Place a \"sample\" order [urgent]", "outcome": "Order confirmed"},
	}}
	res := run(t, "workflow", Input{Kind: "workflow", Metadata: meta})
	d := find(res.Findings, "diagram", "Workflow diagram")
	if d == nil {
		t.Fatal("diagram missing")
	}
	mm := d.Data["mermaid"].(string)
	if !strings.HasPrefix(mm, "flowchart TD") || strings.Contains(mm, `"sample"`) || strings.Contains(mm, "[urgent]") || !strings.Contains(mm, "-- yes -->") {
		t.Errorf("mermaid output unsafe or wrong:\n%s", mm)
	}
	if find(res.Findings, "requirement", "must allow buyer to request a quotation") == nil {
		t.Error("requirement not derived")
	}
	if find(res.Findings, "entity", "Entity: Price list") == nil {
		t.Error("entity not inferred")
	}
	text := StepsFromMetadata(nil, "1. Clerk: Receive goods -> Stock updated\n2. Manager: Approve invoice")
	if len(text) != 2 || text[0].Actor != "Clerk" || text[0].Outcome != "Stock updated" {
		t.Errorf("text step parsing wrong: %+v", text)
	}
}

func TestReportsCiteEvidenceAndSeparateUnknowns(t *testing.T) {
	web := run(t, "website", Input{Kind: "html", URL: "https://shop.northwind.example/p", Text: fixture(t, "product-jsonld.html")})
	api := run(t, "openapi", Input{Kind: "openapi", Text: fixture(t, "openapi-inventory.yaml")})
	var rf []ReportFinding
	for i, f := range append(web.Findings, api.Findings...) {
		rf = append(rf, ReportFinding{ID: strings.Repeat("0", 6) + string(rune('a'+i%26)) + strings.Repeat("1", 25), Category: f.Category,
			Title: f.Title, Detail: f.Detail, Evidence: f.Evidence, Classification: f.Classification, Confidence: f.Confidence,
			Assumptions: f.Assumptions, Data: f.Data, SourceTitle: "fixture"})
	}
	for i := range rf {
		rf[i].ID = rf[i].ID[:5] + string(rune('a'+i%26)) + rf[i].ID[6:]
		rf[i].ID = string(rune('a'+i/26)) + rf[i].ID[1:]
	}
	in := ReportInput{ProjectName: "Northwind study", Objective: "Understand the storefront", AuthorizationBasis: "public_information",
		Sources: []ReportSource{{"1", "Product page", "html"}, {"2", "Inventory API", "openapi"}}, Findings: rf, GeneratedAt: time.Unix(0, 0)}
	for _, kind := range ReportKinds {
		title, md, used, err := GenerateReport(kind, in)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if title == "" || !strings.Contains(md, "deterministic report generator") {
			t.Errorf("%s: missing header", kind)
		}
		if kind != "workflow" && (len(used) == 0 || !strings.Contains(md, "## Evidence index")) {
			t.Errorf("%s: no citations", kind)
		}
	}
	_, plan, _, _ := GenerateReport("reconstruction_plan", in)
	for _, want := range []string{"original", "Missing information and unknowns", "Server-side implementation is not observable", "Phase 1"} {
		if !strings.Contains(plan, want) {
			t.Errorf("reconstruction plan missing %q", want)
		}
	}
	_, inv, _, _ := GenerateReport("api_inventory", in)
	if !strings.Contains(inv, "| GET | `/items/{itemId}` |") {
		t.Errorf("api inventory table wrong:\n%s", inv)
	}
	_, dm, _, _ := GenerateReport("data_model", in)
	if !strings.Contains(dm, "erDiagram") || !strings.Contains(dm, "Item ||--o| Location") {
		t.Errorf("data model diagram wrong:\n%s", dm)
	}
	if _, _, _, err := GenerateReport("nope", in); err == nil {
		t.Error("unknown kind should fail")
	}
}
