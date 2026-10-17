package httpapi_test

import (
	"encoding/json"
	"strings"
	"testing"
)

func rulesDoc(title string) map[string]any {
	return map[string]any{
		"schema":         1,
		"product_signal": map[string]any{"any": []any{map[string]any{"css": "h1.pdp-title"}}},
		"sources":        map[string]any{"state": map[string]any{"kind": "script_marker", "marker": "window.__SHOP__"}},
		"fields": map[string]any{
			"title": []any{map[string]any{"source": "state", "path": "product.name"}, map[string]any{"css": title}},
			"price": []any{map[string]any{"source": "state", "path": "product.price"}},
		},
	}
}

func adapterByKey(t *testing.T, items []map[string]any, key string) map[string]any {
	t.Helper()
	for _, a := range items {
		if a["key"] == key {
			return a
		}
	}
	return nil
}

func TestShippedMarketplaceDefinitions(t *testing.T) {
	e := newEnv(t)
	c := e.owner
	items := c.Do("GET", "/api/v1/adapters", nil).Must(t, 200).Items()
	rules := 0
	for _, a := range items {
		if a["kind"] == "rules" && a["shipped"] == true {
			rules++
			v, _ := a["verification"].(map[string]any)
			if v["status"] != "synthetic_fixture" || a["definition_version"] != float64(1) || a["region"] == "" || a["rules"] == nil {
				t.Fatalf("%s: incomplete shipped definition: %v", a["key"], a)
			}
		}
	}
	if rules != 31 {
		t.Fatalf("expected 31 shipped marketplace definitions, got %d", rules)
	}
	amazon := adapterByKey(t, items, "amazon")
	id := amazon["id"].(string)

	// Viewers can see marketplaces but not change them.
	viewer := e.userWithRole("viewer", "viewer@example.com")
	if len(viewer.Do("GET", "/api/v1/adapters", nil).Must(t, 200).Items()) != len(items) {
		t.Fatal("viewer should list adapters")
	}
	viewer.Do("PUT", "/api/v1/adapters/"+id, map[string]any{"key": "amazon", "name": "x", "domain_patterns": []string{"x.example"}, "enabled": false, "priority": 1}).Must(t, 403)

	// Administrators switch a definition off and re-prioritise it; its definition stays.
	upd := c.Do("PUT", "/api/v1/adapters/"+id, map[string]any{"key": "amazon", "name": "Mine", "domain_patterns": []string{"evil.example"},
		"enabled": false, "priority": 7, "description": "paused", "rules": rulesDoc("h1")}).Must(t, 200)
	if upd.Get("enabled") != false || upd.Get("priority") != float64(7) || upd.Str("name") != "Amazon" || upd.Str("description") != "paused" {
		t.Fatalf("shipped update: %s", upd.Body)
	}
	if b, _ := json.Marshal(upd.Get("rules")); !strings.Contains(string(b), "productTitle") {
		t.Fatalf("shipped rules replaced: %s", b)
	}
	if !strings.Contains(strings.Join(toStrings(upd.Get("domain_patterns")), ","), "amazon.co.uk") {
		t.Fatalf("shipped domains replaced: %v", upd.Get("domain_patterns"))
	}
	if code := c.Do("DELETE", "/api/v1/adapters/"+id, nil).Must(t, 409).ErrorCode(); code != "adapter_builtin" {
		t.Fatalf("delete code %s", code)
	}

	// /extension/config delivers enabled rules adapters with their rule documents.
	ext, _ := e.pairExtension()
	cfg := ext.Do("GET", "/api/v1/extension/config", nil).Must(t, 200)
	delivered := map[string]map[string]any{}
	for _, a := range cfg.Get("adapters").([]any) {
		m := a.(map[string]any)
		delivered[m["key"].(string)] = m
	}
	if _, ok := delivered["amazon"]; ok {
		t.Fatal("a disabled definition was delivered")
	}
	ali := delivered["aliexpress"]
	if ali == nil || ali["kind"] != "rules" || ali["rules"] == nil {
		t.Fatalf("aliexpress not delivered with its rules: %v", ali)
	}
	if doc, _ := ali["rules"].(map[string]any); doc["schema"] != float64(1) {
		t.Fatalf("rule document not delivered: %v", ali["rules"])
	}
}

func TestCustomRulesAdapters(t *testing.T) {
	e := newEnv(t)
	c := e.owner
	body := map[string]any{"key": "acme-rules", "name": "Acme rules", "kind": "rules", "domain_patterns": []string{"*.acme.example", "acme.example"},
		"enabled": true, "priority": 20, "description": "Acme product pages", "rules": rulesDoc("h1.name")}
	a := c.Do("POST", "/api/v1/adapters", body).Must(t, 201)
	if a.Str("kind") != "rules" || a.Get("shipped") != false || a.Get("rules.fields.title.1.css") != "h1.name" {
		t.Fatalf("created: %s", a.Body)
	}
	c.Do("POST", "/api/v1/adapters", body).Must(t, 409)

	// Server-side validation mirrors the extractor's.
	bad := func(rules any, field string) {
		t.Helper()
		b := map[string]any{"key": "bad-rules", "name": "Bad", "kind": "rules", "domain_patterns": []string{"bad.example"}, "enabled": true, "priority": 1, "rules": rules}
		res := c.Do("POST", "/api/v1/adapters", b).Must(t, 422)
		if f, _ := res.Get("error.fields").(map[string]any); f[field] == nil {
			t.Fatalf("expected a %s error: %s", field, res.Body)
		}
	}
	doc := rulesDoc("h1")
	doc["fields"].(map[string]any)["title"] = []any{map[string]any{"source": "state", "path": "csrfToken"}}
	bad(doc, "rules")
	doc = rulesDoc("h1")
	doc["regex"] = "(a+)+$"
	bad(doc, "rules")
	doc = rulesDoc("h1")
	doc["sources"] = map[string]any{"state": map[string]any{"kind": "fetch", "url": "https://x.example"}}
	bad(doc, "rules")
	bad(nil, "rules")
	bad("not an object", "rules")
	res := c.Do("POST", "/api/v1/adapters", map[string]any{"key": "mixed", "name": "Mixed", "kind": "selector", "domain_patterns": []string{"m.example"},
		"selectors": map[string]string{"title": "h1"}, "rules": rulesDoc("h1")}).Must(t, 422)
	if f, _ := res.Get("error.fields").(map[string]any); f["rules"] == nil {
		t.Fatalf("selector adapters must not carry rules: %s", res.Body)
	}
	c.Do("POST", "/api/v1/adapters", map[string]any{"key": "wrongkind", "name": "W", "kind": "script", "domain_patterns": []string{"w.example"}}).Must(t, 422)

	// Updates replace the rule document of custom adapters; the kind is fixed.
	id := a.Str("id")
	body["rules"] = rulesDoc("h1.title-v2")
	upd := c.Do("PUT", "/api/v1/adapters/"+id, body).Must(t, 200)
	if upd.Get("rules.fields.title.1.css") != "h1.title-v2" || upd.Get("version") != float64(2) {
		t.Fatalf("rules not updated: %s", upd.Body)
	}
	body["kind"] = "selector"
	c.Do("PUT", "/api/v1/adapters/"+id, body).Must(t, 422)

	// Members (no settings.manage) cannot create adapters.
	member := e.userWithRole("member", "member@example.com")
	member.Do("POST", "/api/v1/adapters", map[string]any{"key": "m-rules", "name": "M", "kind": "rules", "domain_patterns": []string{"m.example"}, "rules": rulesDoc("h1")}).Must(t, 403)

	ext, _ := e.pairExtension()
	found := false
	for _, x := range ext.Do("GET", "/api/v1/extension/config", nil).Must(t, 200).Get("adapters").([]any) {
		m := x.(map[string]any)
		found = found || (m["key"] == "acme-rules" && m["rules"] != nil)
	}
	if !found {
		t.Fatal("custom rules adapter not delivered to the extension")
	}
	c.Do("DELETE", "/api/v1/adapters/"+id, nil).Must(t, 204)
}
