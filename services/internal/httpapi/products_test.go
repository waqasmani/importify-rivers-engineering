package httpapi_test

import (
	"fmt"
	"strings"
	"testing"
)

func productPayload(sku string) map[string]any {
	return map[string]any{
		"title": "Stainless Steel Water Bottle 750ml", "url": "https://shop.example.com/products/bottle-750?utm_source=newsletter",
		"brand": "HydroCo", "sku": sku, "price": "$24.99", "availability": "https://schema.org/InStock",
		"description": "Double-wall insulated bottle.", "images": []map[string]any{{"url": "https://cdn.shop.example.com/b1.jpg", "alt": "Front"}},
		"variants":          []map[string]any{{"options": map[string]string{"Color": "Black"}, "price": "$24.99"}, {"options": map[string]string{"Color": "Silver"}, "price": "$26.99"}},
		"specifications":    map[string]string{"Capacity": "750 ml", "Material": "18/8 stainless steel"},
		"extraction_method": "json-ld", "adapter_key": "json-ld", "confidence": 0.9,
		"evidence": map[string]string{"price": "script[type=application/ld+json] offers.price"},
	}
}

func TestExtensionProductImportFlow(t *testing.T) {
	e := newEnv(t)
	ext, _ := e.pairExtension()
	supplier := e.owner.Do("POST", "/api/v1/suppliers", map[string]any{"name": "Shop Example", "website": "https://shop.example.com"}).Must(t, 201).Str("id")

	chk := ext.Do("POST", "/api/v1/imports/products/check", map[string]any{"payload": productPayload("HB-750")}).Must(t, 200)
	if chk.Str("normalized.canonical_url") != "https://shop.example.com/products/bottle-750" || chk.Str("normalized.price") != "24.99" ||
		chk.Get("definite_duplicate") != false || chk.Str("suggested_supplier.id") != supplier {
		t.Fatalf("check response wrong: %s", chk.Body)
	}

	capture := ext.Do("POST", "/api/v1/captures", map[string]any{"url": "https://shop.example.com/products/bottle-750", "title": "Bottle",
		"page_type": "product", "adapter_key": "json-ld", "extracted": productPayload("HB-750"),
		"selections": []map[string]any{{"kind": "text", "text": "Ships in 3 days", "selector": "#shipping"}}}).Must(t, 201)

	first := ext.Do("POST", "/api/v1/imports/products", map[string]any{"payload": productPayload("HB-750"), "captured_page_id": capture.Str("id"),
		"tag_names": []string{"bottles"}}).Must(t, 201)
	pid := first.Str("product.id")
	if first.Str("status") != "imported" || first.Str("product.status") != "draft" || first.Str("product.review_status") != "pending" ||
		first.Str("product.supplier_id") != supplier || first.Str("import.source") != "extension" {
		t.Fatalf("first import wrong: %s", first.Body)
	}
	detail := e.owner.Do("GET", "/api/v1/products/"+pid, nil).Must(t, 200)
	if len(detail.Get("variants").([]any)) != 2 || len(detail.Get("images").([]any)) != 1 || detail.Str("captured_page.id") != capture.Str("id") ||
		len(detail.Get("suppliers").([]any)) != 1 || len(detail.Get("tags").([]any)) != 1 {
		t.Fatalf("product detail incomplete: %s", detail.Body)
	}

	// The same page again is parked as a duplicate instead of creating a second product.
	again := ext.Do("POST", "/api/v1/imports/products", map[string]any{"payload": productPayload("HB-750")}).Must(t, 202)
	if again.Str("status") != "needs_decision" || again.Get("product") != nil || again.Str("duplicates.0.id") != pid {
		t.Fatalf("duplicate not detected: %s", again.Body)
	}
	queue := e.owner.Do("GET", "/api/v1/imports?status=duplicate", nil).Must(t, 200)
	if queue.Get("total") != float64(1) {
		t.Fatalf("duplicate not queued: %s", queue.Body)
	}
	updatedPayload := productPayload("HB-750")
	updatedPayload["price"] = "$19.99"
	updatedPayload["variants"] = []map[string]any{{"options": map[string]string{"Color": "Black"}, "price": "$19.99"}, {"options": map[string]string{"Color": "Green"}, "price": "$21.00"}}
	res := ext.Do("POST", "/api/v1/imports/products", map[string]any{"payload": updatedPayload, "decision": "update_existing", "target_product_id": pid}).Must(t, 201)
	if res.Str("status") != "updated" || res.Str("product.price") != "19.99" {
		t.Fatalf("update existing failed: %s", res.Body)
	}
	vars := e.owner.Do("GET", "/api/v1/products/"+pid, nil).Must(t, 200).Get("variants").([]any)
	if len(vars) != 3 {
		t.Fatalf("variants should be matched by options and new ones added, got %d", len(vars))
	}
	// Resolve the parked duplicate by skipping it; resolving twice fails.
	impID := queue.Items()[0]["id"].(string)
	e.owner.Do("POST", "/api/v1/imports/"+impID+"/resolve", map[string]any{"decision": "skip"}).Must(t, 200)
	e.owner.Do("POST", "/api/v1/imports/"+impID+"/resolve", map[string]any{"decision": "skip"}).Must(t, 409)
	// Explicit create_new makes a separate product.
	ext.Do("POST", "/api/v1/imports/products", map[string]any{"payload": productPayload("HB-750"), "decision": "create_new"}).Must(t, 201)
	if e.owner.Do("GET", "/api/v1/products", nil).Get("total") != float64(2) {
		t.Fatal("create_new did not create a second product")
	}
}

func TestImportValidationAndCredentials(t *testing.T) {
	e := newEnv(t)
	ext, _ := e.pairExtension()
	bad := ext.Do("POST", "/api/v1/imports/products", map[string]any{"payload": map[string]any{"title": "", "url": "javascript:alert(1)"}}).Must(t, 422)
	if bad.Get("error.fields.title") == nil || bad.Get("error.fields.url") == nil {
		t.Fatalf("expected title and url errors: %s", bad.Body)
	}
	ext.Do("POST", "/api/v1/imports/products", `{"payload": {"title": "x",`).Must(t, 400)
	ext.Do("POST", "/api/v1/imports/products", map[string]any{"payload": productPayload("A"), "decision": "update_existing"}).Must(t, 422)
	missing := productPayload("NOPRICE")
	delete(missing, "price")
	delete(missing, "images")
	delete(missing, "variants")
	chk := ext.Do("POST", "/api/v1/imports/products/check", map[string]any{"payload": missing}).Must(t, 200)
	warn := strings.Join(toStrings(chk.Get("warnings")), "|")
	if !strings.Contains(warn, "no price") || !strings.Contains(warn, "no product images") {
		t.Fatalf("missing-field warnings absent: %s", warn)
	}
	expired := e.client()
	expired.Bearer = "reaa_not-a-real-token"
	res := expired.Do("POST", "/api/v1/imports/products", map[string]any{"payload": productPayload("A")}).Must(t, 401)
	if res.ErrorCode() != "token_expired" {
		t.Fatalf("expected token_expired: %s", res.Body)
	}
}

func toStrings(v any) []string {
	arr, _ := v.([]any)
	out := make([]string, 0, len(arr))
	for _, x := range arr {
		s, _ := x.(string)
		out = append(out, s)
	}
	return out
}

func TestProductReviewAndManualEdits(t *testing.T) {
	e := newEnv(t)
	c := e.owner
	p := c.Do("POST", "/api/v1/products", map[string]any{"title": "Desk Lamp", "price": "40", "price_currency": "EUR", "cost": "12.50",
		"cost_currency": "EUR", "gtin": "4006381333931"}).Must(t, 201)
	id := p.Str("id")
	if p.Str("margin_percent") != "68.75" || p.Str("margin_amount") != "27.5" {
		t.Fatalf("margin wrong: %s", p.Body)
	}
	c.Do("POST", "/api/v1/products", map[string]any{"title": "Bad", "price": "10"}).Must(t, 422)
	c.Do("POST", "/api/v1/products", map[string]any{"title": "Bad", "gtin": "4006381333932"}).Must(t, 422)
	c.Do("PUT", "/api/v1/products/"+id, map[string]any{"title": "Desk Lamp", "status": "active"}).Must(t, 422)
	member := e.userWithRole("member", "member@example.com")
	member.Do("POST", "/api/v1/products/"+id+"/review", map[string]any{"review_status": "approved"}).Must(t, 403)
	rv := c.Do("POST", "/api/v1/products/"+id+"/review", map[string]any{"review_status": "approved", "note": "Checked specs"}).Must(t, 200)
	if rv.Str("status") != "active" {
		t.Fatalf("approval should activate: %s", rv.Body)
	}
	v := c.Do("POST", "/api/v1/products/"+id+"/variants", map[string]any{"title": "Warm white", "price": "42", "currency": "EUR"}).Must(t, 201)
	c.Do("PUT", "/api/v1/products/"+id+"/variants/"+v.Str("id"), map[string]any{"title": "Warm white", "price": "41", "currency": "EUR"}).Must(t, 200)
	img := c.Do("POST", "/api/v1/products/"+id+"/images", map[string]any{"url": "https://img.example/lamp.jpg"}).Must(t, 201)
	if img.Get("is_primary") != true {
		t.Fatal("first image should be primary")
	}
	c.Do("POST", "/api/v1/products/"+id+"/images", map[string]any{"url": "file:///etc/passwd"}).Must(t, 422)
	sup := c.Do("POST", "/api/v1/suppliers", map[string]any{"name": "Lamp Works"}).Must(t, 201).Str("id")
	c.Do("PUT", "/api/v1/suppliers/"+sup+"/products/"+id, map[string]any{"supplier_sku": "LW-1", "cost": "11", "currency": "EUR", "moq": 100}).Must(t, 200)
	if c.Do("GET", "/api/v1/products/"+id, nil).Str("supplier_id") != sup {
		t.Fatal("supplier not linked to product")
	}
	notes := c.Do("GET", "/api/v1/notes?entity_type=product&entity_id="+id, nil).Must(t, 200)
	if len(notes.Items()) != 1 {
		t.Fatal("review note missing")
	}
}

func TestProductMergeAndBulk(t *testing.T) {
	e := newEnv(t)
	c := e.owner
	a := c.Do("POST", "/api/v1/products", map[string]any{"title": "USB-C Cable 1m"}).Must(t, 201).Str("id")
	b := c.Do("POST", "/api/v1/products", map[string]any{"title": "USB-C Cable 1 m", "brand": "Cabl", "sku": "C1"}).Must(t, 201).Str("id")
	c.Do("POST", "/api/v1/products/"+b+"/images", map[string]any{"url": "https://img.example/c.jpg"}).Must(t, 201)
	dups := c.Do("GET", "/api/v1/products/"+a+"/duplicates", nil).Must(t, 200)
	if len(dups.Items()) != 1 {
		t.Fatalf("similar title not detected: %s", dups.Body)
	}
	m := c.Do("POST", "/api/v1/products/"+a+"/merge", map[string]any{"source_ids": []string{b}}).Must(t, 200)
	if m.Str("brand") != "Cabl" || len(m.Get("images").([]any)) != 1 {
		t.Fatalf("merge wrong: %s", m.Body)
	}
	c.Do("POST", "/api/v1/products/bulk", map[string]any{"action": "set_status", "ids": []string{a}, "value": "active"}).Must(t, 422)
	c.Do("POST", "/api/v1/products/bulk", map[string]any{"action": "set_review", "ids": []string{a}, "value": "approved"}).Must(t, 200)
	if c.Do("GET", "/api/v1/products/"+a, nil).Str("status") != "active" {
		t.Fatal("bulk approval should activate drafts")
	}
	csv := "Product Name,SKU,Price,Currency,Supplier,Image\nTravel Mug,TM-1,12.50,usd,Mug Factory,https://img.example/m.jpg\n"
	res := c.Upload("/api/v1/import/csv/commit", "file", "p.csv", []byte(csv), map[string]string{"entity_type": "product"}).Must(t, 200)
	if res.Get("created") != float64(1) {
		t.Fatalf("product csv import failed: %s", res.Body)
	}
	mug := c.Do("GET", "/api/v1/products?q=travel", nil).Must(t, 200).Items()
	if len(mug) != 1 || mug[0]["supplier_name"] != "Mug Factory" || mug[0]["price_currency"] != "USD" {
		t.Fatalf("csv product wrong: %v", mug)
	}
	exp := c.Do("GET", "/api/v1/products/export?format=csv", nil).Must(t, 200)
	if !strings.Contains(string(exp.Body), "Travel Mug") {
		t.Fatal("product export missing data")
	}
}

func TestCaptureSnapshotsRespectSettings(t *testing.T) {
	e := newEnv(t)
	ext, _ := e.pairExtension()
	snap := `<html><body><input type="password" value="hunter2"><script>x()</script><h1>Item</h1></body></html>`
	cap1 := ext.Do("POST", "/api/v1/captures", map[string]any{"url": "https://a.example/x", "title": "X", "html_snapshot": snap}).Must(t, 201)
	if !strings.Contains(string(cap1.Body), "snapshot storage is disabled") {
		t.Fatalf("expected discard warning: %s", cap1.Body)
	}
	e.owner.Do("PUT", "/api/v1/settings/capture", map[string]any{"store_html_snapshots": true, "max_snapshot_kb": 64, "retention_days": 0}).Must(t, 200)
	cap2 := ext.Do("POST", "/api/v1/captures", map[string]any{"url": "https://a.example/x", "title": "X", "html_snapshot": snap}).Must(t, 201)
	s := e.owner.Do("GET", "/api/v1/captures/"+cap2.Str("id")+"/snapshot", nil).Must(t, 200)
	if strings.Contains(string(s.Body), "hunter2") || strings.Contains(string(s.Body), "x()") || !strings.HasPrefix(s.Header.Get("Content-Type"), "text/plain") {
		t.Fatalf("snapshot not sanitized or served unsafely: %s %v", s.Body, s.Header)
	}
	ext.Do("POST", "/api/v1/captures", map[string]any{"url": "notaurl", "title": "X"}).Must(t, 422)
	list := e.owner.Do("GET", "/api/v1/captures?domain=a.example", nil).Must(t, 200)
	if list.Get("total") != float64(2) {
		t.Fatalf("capture list wrong: %s", list.Body)
	}
	purge := e.owner.Do("POST", "/api/v1/captures/purge", map[string]any{"before": "2999-01-01T00:00:00Z"}).Must(t, 422)
	_ = purge
}

func TestSelectorAdapters(t *testing.T) {
	e := newEnv(t)
	c := e.owner
	ad := c.Do("POST", "/api/v1/adapters", map[string]any{"key": "acme-store", "name": "Acme store", "domain_patterns": []string{"*.acme.example"},
		"enabled": true, "priority": 5, "selectors": map[string]string{"title": "h1.product-name", "price": ".price-now"}}).Must(t, 201)
	c.Do("POST", "/api/v1/adapters", map[string]any{"key": "bad", "name": "Bad", "domain_patterns": []string{"*"}, "selectors": map[string]string{"title": "<script>"}}).Must(t, 422)
	ext, _ := e.pairExtension()
	cfg := ext.Do("GET", "/api/v1/extension/config", nil).Must(t, 200)
	if cfg.Str("user.role_key") != "admin" || cfg.Str("user.role_name") == "" {
		t.Fatalf("extension config must name the user's role: %s", cfg.Body)
	}
	found := false
	for _, a := range cfg.Get("adapters").([]any) {
		found = found || a.(map[string]any)["key"] == "acme-store"
	}
	if !found {
		t.Fatal("selector adapter not delivered to extension")
	}
	var builtin string
	for _, a := range c.Do("GET", "/api/v1/adapters", nil).Must(t, 200).Items() {
		if a["kind"] == "builtin" {
			builtin = a["id"].(string)
		}
	}
	c.Do("DELETE", "/api/v1/adapters/"+builtin, nil).Must(t, 409)
	c.Do("DELETE", "/api/v1/adapters/"+ad.Str("id"), nil).Must(t, 204)

	// Every built-in adapter can be toggled, including wildcard ones such as json-ld, and keeps
	// its domains and extraction definition.
	for _, a := range c.Do("GET", "/api/v1/adapters", nil).Must(t, 200).Items() {
		if a["kind"] != "builtin" {
			continue
		}
		id := a["id"].(string)
		upd := c.Do("PUT", "/api/v1/adapters/"+id, map[string]any{"key": a["key"], "name": "Renamed", "domain_patterns": []string{"evil.example"},
			"enabled": false, "priority": 7, "selectors": map[string]string{"title": "h1"}, "description": "off for now"}).Must(t, 200)
		if upd.Get("enabled") != false || upd.Get("priority") != float64(7) || upd.Str("description") != "off for now" {
			t.Fatalf("%s: toggle not applied: %v", a["key"], upd.Body)
		}
		if upd.Str("name") != a["name"] || fmt.Sprint(upd.Get("domain_patterns")) != fmt.Sprint(a["domain_patterns"]) || fmt.Sprint(upd.Get("selectors")) != fmt.Sprint(a["selectors"]) {
			t.Fatalf("%s: built-in definition changed: before %v after %v", a["key"], a, upd.Body)
		}
	}
}

func TestImportCanSkipSuggestedSupplier(t *testing.T) {
	e := newEnv(t)
	e.owner.Do("POST", "/api/v1/suppliers", map[string]any{"name": "Shop Example", "website": "https://shop.example.com"}).Must(t, 201)
	res := e.owner.Do("POST", "/api/v1/imports/products", map[string]any{"payload": productPayload("SKIP-1"), "source": "manual",
		"skip_supplier_link": true}).Must(t, 201)
	if res.Get("product.supplier_id") != nil {
		t.Fatalf("supplier was linked although the user chose none: %s", res.Body)
	}
	e.owner.Do("POST", "/api/v1/imports/products", map[string]any{"payload": productPayload("SKIP-2"), "source": "manual",
		"skip_supplier_link": true, "create_supplier": true}).Must(t, 422)

	// Pipeline summaries count leads and suppliers on their own pipelines, not only opportunities.
	checked := false
	for _, pl := range e.owner.Do("GET", "/api/v1/pipelines", nil).Must(t, 200).Items() {
		if pl["entity_type"] != "lead" || pl["is_default"] != true {
			continue
		}
		stage := pl["stages"].([]any)[0].(map[string]any)["id"].(string)
		e.owner.Do("POST", "/api/v1/leads", map[string]any{"title": "Counted lead", "pipeline_stage_id": stage, "estimated_value": "150.00", "currency": "EUR"}).Must(t, 201)
		sum := e.owner.Do("GET", "/api/v1/pipelines/"+pl["id"].(string)+"/summary", nil).Must(t, 200)
		first := sum.Get("stages").([]any)[0].(map[string]any)
		if first["deal_count"] != float64(1) || first["total_amount"] != "150" {
			t.Fatalf("lead pipeline summary wrong: %v", first)
		}
		checked = true
	}
	if !checked {
		t.Fatal("no default lead pipeline found")
	}
}
