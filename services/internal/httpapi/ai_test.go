package httpapi_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeOllama answers /api/chat with canned JSON chosen by the requested schema.
func fakeOllama(t *testing.T, fail *atomic.Int32) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail != nil && fail.Load() > 0 {
			fail.Add(-1)
			w.WriteHeader(500)
			_, _ = w.Write([]byte(`{"error":"model crashed"}`))
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			Format struct {
				Properties map[string]any `json:"properties"`
			} `json:"format"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.Unmarshal(raw, &body)
		props := body.Format.Properties
		var out any
		switch {
		case props["specifications"] != nil:
			out = map[string]any{"title": map[string]any{"value": "Insulated Flask 2 L", "citations": []string{"E1"}},
				"category": map[string]any{"value": "Outdoor > Drinkware", "citations": []string{"E1"}},
				"brand": map[string]any{"value": "Northwind", "citations": []string{"E1"}},
				"description": map[string]any{"value": "A 2 litre stainless steel flask.", "citations": []string{"E1", "E99"}},
				"specifications": []any{map[string]any{"name": "Capacity", "value": "2 L", "citations": []string{"E1"}}}}
		case props["subject"] != nil:
			out = map[string]any{"subject": "Samples for order", "message": "Hello, could you confirm the MOQ? [E1]", "questions": []any{},
				"suggested_task_title": "Chase samples", "due_in_days": 2}
		case props["markdown"] != nil:
			out = map[string]any{"title": "Storefront documentation", "markdown": "# Overview\nThe page sells a flask [E2].", "unknowns": []string{"Backend stack"}}
		case props["key_points"] != nil:
			out = map[string]any{"summary": "A supplier record.", "key_points": []any{map[string]any{"text": "Located in Ningbo", "citations": []string{"E1"}}}, "unknowns": []string{}}
		case props["status"] != nil:
			out = map[string]any{"status": "ok"}
		default:
			out = map[string]any{}
		}
		content, _ := json.Marshal(out)
		reply, _ := json.Marshal(map[string]any{"model": "fake-llm", "message": map[string]any{"role": "assistant", "content": string(content)},
			"prompt_eval_count": 100, "eval_count": 20})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(reply)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAIDisabledByDefault(t *testing.T) {
	e := newEnv(t)
	st := e.owner.Do("GET", "/api/v1/ai/status", nil).Must(t, 200)
	if st.Get("configured") != false || !strings.Contains(st.Str("message"), "works without AI") {
		t.Fatalf("status wrong: %s", st.Body)
	}
	pid := e.owner.Do("POST", "/api/v1/products", map[string]any{"title": "Kettle"}).Must(t, 201).Str("id")
	res := e.owner.Do("POST", "/api/v1/ai/jobs", map[string]any{"tool": "normalize_product", "entity_type": "product", "entity_id": pid}).Must(t, 409)
	if res.ErrorCode() != "ai_not_configured" {
		t.Fatalf("expected ai_not_configured, got %s", res.ErrorCode())
	}
	tools := e.owner.Do("GET", "/api/v1/ai/tools", nil).Must(t, 200)
	if len(tools.Items()) < 12 {
		t.Fatalf("tool catalogue incomplete: %d", len(tools.Items()))
	}
}

func TestAIProviderSecretsAndExternalControls(t *testing.T) {
	e := newEnv(t)
	c := e.owner
	c.Do("POST", "/api/v1/ai/providers", map[string]any{"name": "Plain HTTP cloud", "kind": "openai_compatible", "base_url": "http://llm.example.com/v1",
		"model": "m", "enabled": true}).Must(t, 422)
	c.Do("POST", "/api/v1/ai/providers", map[string]any{"name": "No key", "kind": "anthropic", "model": "claude-opus-5-5", "enabled": true}).Must(t, 422)
	ext := c.Do("POST", "/api/v1/ai/providers", map[string]any{"name": "Claude", "kind": "anthropic", "model": "claude-opus-5-5",
		"api_key": "sk-ant-very-secret", "enabled": true, "is_default": true}).Must(t, 201)
	if ext.Get("has_api_key") != true || ext.Get("is_local") != false || ext.Get("allow_external_data") != false || strings.Contains(string(ext.Body), "very-secret") {
		t.Fatalf("provider view wrong or leaks key: %s", ext.Body)
	}
	list := c.Do("GET", "/api/v1/ai/providers", nil).Must(t, 200)
	if strings.Contains(string(list.Body), "very-secret") || strings.Contains(string(list.Body), "ciphertext") {
		t.Fatal("provider list leaks the key")
	}
	var stored []byte
	_ = e.db.Pool.QueryRow(t.Context(), "SELECT api_key_ciphertext FROM ai_provider_configs WHERE id = $1", ext.Str("id")).Scan(&stored)
	if strings.Contains(string(stored), "very-secret") {
		t.Fatal("API key stored in plaintext")
	}
	logs := c.Do("GET", "/api/v1/audit-logs", nil).Must(t, 200)
	if strings.Contains(string(logs.Body), "very-secret") {
		t.Fatal("API key leaked into audit log")
	}

	cid := c.Do("POST", "/api/v1/companies", map[string]any{"name": "Private Co"}).Must(t, 201).Str("id")
	sid := c.Do("POST", "/api/v1/suppliers", map[string]any{"name": "Private Supplier"}).Must(t, 201).Str("id")
	_ = cid
	r1 := c.Do("POST", "/api/v1/ai/jobs", map[string]any{"tool": "summarize", "entity_type": "supplier", "entity_id": sid}).Must(t, 409)
	if r1.ErrorCode() != "external_data_not_allowed" {
		t.Fatalf("expected external_data_not_allowed: %s", r1.Body)
	}
	c.Do("PUT", "/api/v1/ai/providers/"+ext.Str("id"), map[string]any{"name": "Claude", "kind": "anthropic", "model": "claude-opus-5-5",
		"enabled": true, "allow_external_data": true}).Must(t, 200)
	r2 := c.Do("POST", "/api/v1/ai/jobs", map[string]any{"tool": "summarize", "entity_type": "supplier", "entity_id": sid}).Must(t, 409)
	if r2.ErrorCode() != "external_confirmation_required" || r2.Str("provider.host") != "api.anthropic.com" {
		t.Fatalf("expected confirmation request: %s", r2.Body)
	}
	r3 := c.Do("POST", "/api/v1/ai/jobs", map[string]any{"tool": "summarize", "entity_type": "supplier", "entity_id": sid, "confirm_external": true}).Must(t, 202)
	if r3.Get("external_data_confirmed") != true {
		t.Fatalf("confirmation not recorded: %s", r3.Body)
	}
	viewer := e.userWithRole("viewer", "v@example.com")
	viewer.Do("POST", "/api/v1/ai/jobs", map[string]any{"tool": "summarize", "entity_type": "supplier", "entity_id": sid}).Must(t, 403)
	viewer.Do("GET", "/api/v1/ai/providers", nil).Must(t, 403)
}

func TestAIJobLifecycleWithLocalProvider(t *testing.T) {
	e := newEnv(t)
	c := e.owner
	srv := fakeOllama(t, nil)
	prov := c.Do("POST", "/api/v1/ai/providers", map[string]any{"name": "Local", "kind": "ollama", "base_url": srv.URL, "model": "llama3.3", "enabled": true}).Must(t, 201)
	if prov.Get("is_local") != true || prov.Get("is_default") != true {
		t.Fatalf("local provider wrong: %s", prov.Body)
	}
	test := c.Do("POST", "/api/v1/ai/providers/"+prov.Str("id")+"/test", map[string]any{}).Must(t, 200)
	if test.Get("ok") != true {
		t.Fatalf("connection test failed: %s", test.Body)
	}

	pid := c.Do("POST", "/api/v1/products", map[string]any{"title": "TB2L flask!!! BEST deal", "description": "flask"}).Must(t, 201).Str("id")
	job := c.Do("POST", "/api/v1/ai/jobs", map[string]any{"tool": "normalize_product", "entity_type": "product", "entity_id": pid}).Must(t, 202)
	e.worker.Drain(t.Context())
	done := c.Do("GET", "/api/v1/ai/jobs/"+job.Str("id"), nil).Must(t, 200)
	if done.Str("job.status") != "succeeded" || done.Str("job.model") != "fake-llm" {
		t.Fatalf("job not done: %s", done.Body)
	}
	if !strings.Contains(string(done.Body), `"invalid_refs":["E99"]`) {
		t.Fatalf("grounding did not flag invented citation: %s", done.Str("job.grounding"))
	}
	ap := c.Do("POST", "/api/v1/ai/jobs/"+job.Str("id")+"/apply", map[string]any{"fields": []string{"title", "category", "specifications"}}).Must(t, 200)
	if ap.Str("product.title") != "Insulated Flask 2 L" || ap.Str("product.category") != "Outdoor > Drinkware" || ap.Str("product.brand") != "" {
		t.Fatalf("apply did not respect selected fields: %s", ap.Body)
	}
	c.Do("POST", "/api/v1/ai/jobs/"+job.Str("id")+"/apply", map[string]any{}).Must(t, 409)

	// Supplier follow-up creates a draft task; nothing is sent.
	sid := c.Do("POST", "/api/v1/suppliers", map[string]any{"name": "Ningbo Flasks"}).Must(t, 201).Str("id")
	c.Do("POST", "/api/v1/notes", map[string]any{"entity_type": "supplier", "entity_id": sid, "body": "Asked for samples on Monday"}).Must(t, 201)
	fj := c.Do("POST", "/api/v1/ai/jobs", map[string]any{"tool": "supplier_followup", "entity_type": "supplier", "entity_id": sid}).Must(t, 202)
	e.worker.Drain(t.Context())
	fa := c.Do("POST", "/api/v1/ai/jobs/"+fj.Str("id")+"/apply", map[string]any{}).Must(t, 200)
	if fa.Str("task.task_type") != "follow_up" || !strings.Contains(fa.Str("task.description"), "NOT sent") {
		t.Fatalf("follow-up apply wrong: %s", fa.Body)
	}

	// AI documentation becomes a report clearly marked as generated.
	pjt := e.project()
	e.owner.Do("POST", "/api/v1/analysis/projects/"+pjt+"/sources", map[string]any{"kind": "html", "content": readFixture(t, "product-jsonld.html")}).Must(t, 201)
	e.worker.Drain(t.Context())
	dj := c.Do("POST", "/api/v1/ai/jobs", map[string]any{"tool": "generate_tech_doc", "entity_type": "analysis_project", "entity_id": pjt}).Must(t, 202)
	e.worker.Drain(t.Context())
	da := c.Do("POST", "/api/v1/ai/jobs/"+dj.Str("id")+"/apply", map[string]any{}).Must(t, 200)
	if da.Get("report.is_ai_generated") != true || !strings.Contains(da.Str("report.content_markdown"), "AI-generated content") ||
		!strings.Contains(da.Str("report.content_markdown"), "## Evidence index") || len(da.Get("report.finding_ids").([]any)) != 1 {
		t.Fatalf("AI report wrong: %s", da.Body)
	}
	// Informational tools cannot be applied.
	other := c.Do("POST", "/api/v1/products", map[string]any{"title": "TB2L flask BEST deal"}).Must(t, 201).Str("id")
	cmp := c.Do("POST", "/api/v1/ai/jobs", map[string]any{"tool": "compare_products", "entity_type": "product", "entity_id": pid, "params": map[string]any{"product_ids": []string{other}}}).Must(t, 202)
	e.worker.Drain(t.Context())
	c.Do("POST", "/api/v1/ai/jobs/"+cmp.Str("id")+"/apply", map[string]any{}).Must(t, 409)
	c.Do("POST", "/api/v1/ai/jobs", map[string]any{"tool": "compare_products", "entity_type": "product", "entity_id": pid}).Must(t, 422)
	c.Do("POST", "/api/v1/ai/jobs", map[string]any{"tool": "normalize_product", "entity_type": "company", "entity_id": pid}).Must(t, 422)
}

func TestAIProviderFailureIsGraceful(t *testing.T) {
	e := newEnv(t)
	var fail atomic.Int32
	fail.Store(10)
	srv := fakeOllama(t, &fail)
	e.owner.Do("POST", "/api/v1/ai/providers", map[string]any{"name": "Flaky", "kind": "ollama", "base_url": srv.URL, "model": "m", "enabled": true}).Must(t, 201)
	sid := e.owner.Do("POST", "/api/v1/suppliers", map[string]any{"name": "Any supplier"}).Must(t, 201).Str("id")
	job := e.owner.Do("POST", "/api/v1/ai/jobs", map[string]any{"tool": "summarize", "entity_type": "supplier", "entity_id": sid}).Must(t, 202)
	e.worker.Drain(t.Context())
	j := e.owner.Do("GET", "/api/v1/ai/jobs/"+job.Str("id"), nil).Must(t, 200)
	if j.Str("job.status") != "failed" || j.Get("job.attempts") != float64(2) || !strings.Contains(j.Str("job.error"), "500") {
		t.Fatalf("failure not handled: %s", j.Body)
	}
	// The rest of the product keeps working while the provider is down.
	e.owner.Do("POST", "/api/v1/companies", map[string]any{"name": "Still works"}).Must(t, 201)
	e.owner.Do("PUT", "/api/v1/settings/ai", map[string]any{"enabled": false}).Must(t, 200)
	e.owner.Do("POST", "/api/v1/ai/jobs", map[string]any{"tool": "summarize", "entity_type": "supplier", "entity_id": sid}).Must(t, 409)
}
