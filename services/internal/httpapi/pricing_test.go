package httpapi_test

import (
	"strings"
	"testing"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/testutil"
)

// defaultRuleID returns the id of the seeded fallback rule.
func defaultRuleID(t *testing.T, c *testutil.Client) string {
	t.Helper()
	for _, r := range c.Do("GET", "/api/v1/pricing/rules", nil).Must(t, 200).Items() {
		if r["is_default"] == true {
			return r["id"].(string)
		}
	}
	t.Fatal("default pricing rule missing")
	return ""
}

func setDefaultRule(t *testing.T, c *testutil.Client, body map[string]any) {
	t.Helper()
	body["name"] = "Default rule"
	c.Do("PUT", "/api/v1/pricing/rules/"+defaultRuleID(t, c), body).Must(t, 200)
}

func eurPayload(sku, price string, variants ...string) map[string]any {
	p := map[string]any{
		"title": "Enamel Camping Mug " + sku, "url": "https://shop.example.com/products/mug-" + strings.ToLower(sku), "sku": sku,
		"price": price, "images": []map[string]any{{"url": "https://cdn.shop.example.com/mug.jpg"}}, "category": "Outdoor > Cookware",
	}
	vs := []map[string]any{}
	for i, vp := range variants {
		vs = append(vs, map[string]any{"options": map[string]string{"Size": []string{"S", "L", "XL"}[i]}, "price": vp})
	}
	if len(vs) > 0 {
		p["variants"] = vs
	}
	return p
}

func TestPricingRulesRatesAndPreview(t *testing.T) {
	e := newEnv(t)
	c := e.owner
	list := c.Do("GET", "/api/v1/pricing/rules", nil).Must(t, 200)
	if list.Str("base_currency") != "USD" || len(list.Items()) != 1 || list.Get("items.0.is_default") != true || list.Str("items.0.markup_percent") != "" {
		t.Fatalf("seeded rules wrong: %s", list.Body)
	}

	member := e.userWithRole("member", "member@example.com")
	member.Do("GET", "/api/v1/pricing/rules", nil).Must(t, 403)
	member.Do("PUT", "/api/v1/pricing/rates/EUR", map[string]any{"rate": "1.1"}).Must(t, 403)
	member.Do("POST", "/api/v1/pricing/preview", map[string]any{"cost": "10"}).Must(t, 403)

	// Validation.
	bad := c.Do("POST", "/api/v1/pricing/rules", map[string]any{"name": "Bad", "source_domains": []string{"http://x"}, "rounding": "ending",
		"min_cost": "10", "max_cost": "5", "compare_at_mode": "multiplier", "compare_at_value": "0.9", "multiplier": "0",
		"supplier_ids": []string{"00000000-0000-4000-8000-000000000123"}, "min_margin_percent": "100"}).Must(t, 422)
	for _, f := range []string{"source_domains", "rounding_ending", "max_cost", "compare_at_value", "multiplier", "supplier_ids", "min_margin_percent"} {
		if bad.Get("error.fields."+f) == nil {
			t.Errorf("expected a validation error for %s: %s", f, bad.Body)
		}
	}
	c.Do("POST", "/api/v1/pricing/rules", map[string]any{"name": "Bad", "rounding_ending": "0.995", "rounding": "ending"}).Must(t, 422)

	supplier := c.Do("POST", "/api/v1/suppliers", map[string]any{"name": "Mug Works"}).Must(t, 201).Str("id")
	tag := c.Do("POST", "/api/v1/tags", map[string]any{"name": "premium", "color": "#0ea5e9"}).Must(t, 201).Str("id")
	ali := c.Do("POST", "/api/v1/pricing/rules", map[string]any{"name": "AliExpress", "source_domains": []string{"AliExpress.com"},
		"multiplier": "2", "rounding": "ending", "rounding_ending": "0.99", "compare_at_mode": "multiplier", "compare_at_value": "1.3"}).Must(t, 201)
	if ali.Get("priority") != float64(10) || ali.Str("source_domains.0") != "aliexpress.com" || ali.Get("enabled") != true {
		t.Fatalf("created rule wrong: %s", ali.Body)
	}
	cheap := c.Do("POST", "/api/v1/pricing/rules", map[string]any{"name": "Premium tag", "tag_ids": []string{tag}, "supplier_ids": []string{supplier},
		"markup_percent": "80", "fixed_amount": "2.5", "min_price": "9.99", "rounding": "step", "rounding_step": "0.05"}).Must(t, 201)
	if cheap.Get("priority") != float64(20) || cheap.Str("suppliers.0.name") != "Mug Works" || cheap.Str("tags.0.name") != "premium" {
		t.Fatalf("rule view wrong: %s", cheap.Body)
	}

	// Order: reordering needs every non-default rule exactly once.
	c.Do("POST", "/api/v1/pricing/rules/reorder", map[string]any{"ids": []string{ali.Str("id")}}).Must(t, 422)
	re := c.Do("POST", "/api/v1/pricing/rules/reorder", map[string]any{"ids": []string{cheap.Str("id"), ali.Str("id")}}).Must(t, 200)
	if re.Str("items.0.name") != "Premium tag" || re.Str("items.1.name") != "AliExpress" || re.Str("items.2.name") != "Default rule" {
		t.Fatalf("reorder wrong: %s", re.Body)
	}

	// The default rule has no conditions and cannot be deleted.
	def := defaultRuleID(t, c)
	c.Do("PUT", "/api/v1/pricing/rules/"+def, map[string]any{"name": "Default rule", "category_contains": "x"}).Must(t, 422)
	upd := c.Do("PUT", "/api/v1/pricing/rules/"+def, map[string]any{"name": "Default rule", "markup_percent": "40"}).Must(t, 200)
	if upd.Str("markup_percent") != "40" || upd.Get("is_default") != true {
		t.Fatalf("default rule update wrong: %s", upd.Body)
	}
	if c.Do("DELETE", "/api/v1/pricing/rules/"+def, nil).Must(t, 409).ErrorCode() != "default_rule" {
		t.Fatal("default rule must not be deletable")
	}

	// Rates.
	c.Do("PUT", "/api/v1/pricing/rates/USD", map[string]any{"rate": "1"}).Must(t, 422)
	c.Do("PUT", "/api/v1/pricing/rates/EUR", map[string]any{"rate": "0"}).Must(t, 422)
	c.Do("PUT", "/api/v1/pricing/rates/XXX", map[string]any{"rate": "2"}).Must(t, 422)
	rate := c.Do("PUT", "/api/v1/pricing/rates/eur", map[string]any{"rate": "1.0882", "note": "ECB 2026-10-16"}).Must(t, 200)
	if rate.Str("currency") != "EUR" || rate.Str("base_currency") != "USD" || rate.Str("rate") != "1.0882" {
		t.Fatalf("rate wrong: %s", rate.Body)
	}
	bulk := c.Do("PUT", "/api/v1/pricing/rates", map[string]any{"rates": []map[string]any{{"currency": "CNY", "rate": "0.1389"}, {"currency": "EUR", "rate": "1.09"}}}).Must(t, 200)
	if len(bulk.Items()) != 2 || bulk.Str("items.1.rate") != "1.09" {
		t.Fatalf("bulk rates wrong: %s", bulk.Body)
	}
	c.Do("PUT", "/api/v1/pricing/rates", map[string]any{"rates": []map[string]any{{"currency": "CNY", "rate": "1"}, {"currency": "CNY", "rate": "2"}}}).Must(t, 422)

	// Operators can maintain rates from a script with a personal token.
	tok := c.Do("POST", "/api/v1/auth/tokens", map[string]any{"name": "rates cron", "scopes": []string{"settings.manage"}, "expires_in_days": 30}).Must(t, 201).Str("token")
	script := e.client()
	script.Bearer = tok
	script.Do("PUT", "/api/v1/pricing/rates/GBP", map[string]any{"rate": "1.27"}).Must(t, 200)
	if got := c.Do("GET", "/api/v1/pricing/rates", nil).Must(t, 200); len(got.Items()) != 3 || got.Str("base_currency") != "USD" {
		t.Fatalf("rate list wrong: %s", got.Body)
	}
	c.Do("DELETE", "/api/v1/pricing/rates/GBP", nil).Must(t, 204)
	c.Do("DELETE", "/api/v1/pricing/rates/GBP", nil).Must(t, 404)

	// Preview with the saved rules: 10 EUR from AliExpress → 10.90 USD × 2 = 21.80 → 21.99, compare-at 28.59 → 28.99.
	pv := c.Do("POST", "/api/v1/pricing/preview", map[string]any{"cost": "10", "currency": "EUR", "source_domain": "www.aliexpress.com"}).Must(t, 200)
	if pv.Str("status") != "priced" || pv.Str("rule.name") != "AliExpress" || pv.Str("base_cost") != "10.9" || pv.Str("price") != "21.99" ||
		pv.Str("compare_at_price") != "28.99" || pv.Str("steps.0.kind") != "conversion" || pv.Str("currency") != "USD" {
		t.Fatalf("preview wrong: %s", pv.Body)
	}
	// Supplier and tag conditions: (10 × 1.8) + 2.50 = 20.50.
	pt := c.Do("POST", "/api/v1/pricing/preview", map[string]any{"cost": "10", "supplier_id": supplier, "tag_ids": []string{tag}}).Must(t, 200)
	if pt.Str("rule.name") != "Premium tag" || pt.Str("price") != "20.5" {
		t.Fatalf("tag preview wrong: %s", pt.Body)
	}
	// Falls through to the default rule (+40%).
	pd := c.Do("POST", "/api/v1/pricing/preview", map[string]any{"cost": "10"}).Must(t, 200)
	if pd.Str("rule.name") != "Default rule" || pd.Str("price") != "14" || pd.Str("margin_percent") != "28.57" {
		t.Fatalf("default preview wrong: %s", pd.Body)
	}
	if c.Do("POST", "/api/v1/pricing/preview", map[string]any{"cost": "10", "currency": "JPY"}).Must(t, 200).Str("status") != "no_rate" {
		t.Fatal("missing rate should be reported")
	}
	// A draft rule is applied whatever its conditions, which are reported.
	draft := c.Do("POST", "/api/v1/pricing/preview", map[string]any{"cost": "4", "source_domain": "shop.example.com",
		"rule": map[string]any{"name": "Draft", "source_domains": []string{"alibaba.com"}, "max_cost": "5", "multiplier": "3", "min_margin_amount": "10"}}).Must(t, 200)
	if draft.Str("price") != "14" || draft.Get("conditions_met") != false || draft.Str("unmet_conditions.0") != "source domain" {
		t.Fatalf("draft preview wrong: %s", draft.Body)
	}
	invalid := c.Do("POST", "/api/v1/pricing/preview", map[string]any{"cost": "4", "rule": map[string]any{"name": ""}}).Must(t, 422)
	if fields, _ := invalid.Get("error.fields").(map[string]any); fields["rule.name"] == nil {
		t.Fatalf("draft rule errors should be prefixed: %s", invalid.Body)
	}

	c.Do("DELETE", "/api/v1/pricing/rules/"+ali.Str("id"), nil).Must(t, 204)
	logs := c.Do("GET", "/api/v1/audit-logs?entity_type=pricing_rule", nil).Must(t, 200)
	actions := map[string]bool{}
	for _, l := range logs.Items() {
		actions[l["action"].(string)] = true
	}
	for _, a := range []string{"pricing_rule.create", "pricing_rule.update", "pricing_rule.reorder", "pricing_rule.delete"} {
		if !actions[a] {
			t.Errorf("audit event %s missing", a)
		}
	}
}

func TestImportComputesSellingPrices(t *testing.T) {
	e := newEnv(t)
	c := e.owner
	c.Do("PUT", "/api/v1/pricing/rates/EUR", map[string]any{"rate": "1.10"}).Must(t, 200)
	rule := c.Do("POST", "/api/v1/pricing/rules", map[string]any{"name": "Shop markup", "source_domains": []string{"shop.example.com"},
		"markup_percent": "50", "rounding": "ending", "rounding_ending": "0.99", "compare_at_mode": "percent", "compare_at_value": "30"}).Must(t, 201).Str("id")
	ext, _ := e.pairExtension()

	// The review screen sees the computed price before anything is stored.
	chk := ext.Do("POST", "/api/v1/imports/products/check", map[string]any{"payload": eurPayload("M1", "€20.00", "€20.00", "€22.00")}).Must(t, 200)
	if chk.Str("pricing.product.price") != "32.99" || chk.Str("pricing.product.rule.name") != "Shop markup" || chk.Str("pricing.product.base_cost") != "22" ||
		chk.Str("pricing.variants.1.evaluation.price") != "35.99" || chk.Str("pricing.base_currency") != "USD" {
		t.Fatalf("check pricing wrong: %s", chk.Body)
	}

	// 20 EUR → cost 22 USD → +50% = 33 → nearest .99 = 32.99; compare-at 32.99 × 1.3 = 42.89 → 42.99.
	res := ext.Do("POST", "/api/v1/imports/products", map[string]any{"payload": eurPayload("M1", "€20.00", "€20.00", "€22.00")}).Must(t, 201)
	pid := res.Str("product.id")
	p := res.Get("product").(map[string]any)
	want := map[string]any{"source_price": "20", "source_currency": "EUR", "cost": "22", "cost_currency": "USD", "cost_manual": false,
		"price": "32.99", "price_currency": "USD", "compare_at_price": "42.99", "price_locked": false, "pricing_rule_id": rule,
		"margin_amount": "10.99", "margin_percent": "33.31"}
	for k, v := range want {
		if p[k] != v {
			t.Errorf("product.%s = %v, want %v", k, p[k], v)
		}
	}
	detail := c.Do("GET", "/api/v1/products/"+pid, nil).Must(t, 200)
	vars := detail.Get("variants").([]any)
	if len(vars) != 2 {
		t.Fatalf("variants: %s", detail.Body)
	}
	byPrice := map[string]bool{}
	for _, v := range vars {
		m := v.(map[string]any)
		byPrice[m["price"].(string)] = m["pricing_rule_id"] == rule && m["source_currency"] == "EUR" && m["currency"] == "USD"
	}
	if !byPrice["32.99"] || !byPrice["35.99"] {
		t.Fatalf("variant prices wrong: %v", vars)
	}
	if detail.Str("pricing_rule_name") != "Shop markup" || detail.Str("pricing.product.status") != "repriced" || detail.Get("pricing.product.changed") != false ||
		len(detail.Get("pricing.variants").([]any)) != 2 || detail.Str("pricing.rule_names."+rule) != "Shop markup" {
		t.Fatalf("pricing breakdown wrong: %s", detail.Body)
	}
	imp := c.Do("GET", "/api/v1/imports/"+res.Str("import.id"), nil).Must(t, 200)
	if imp.Str("pricing.product.price") != "32.99" {
		t.Fatalf("import detail pricing wrong: %s", imp.Body)
	}

	// A new observation re-derives the cost and reprices: 30 EUR → 33 USD → 49.50 → 49.99.
	up := ext.Do("POST", "/api/v1/imports/products", map[string]any{"payload": eurPayload("M1", "€30.00"), "decision": "update_existing", "target_product_id": pid}).Must(t, 201)
	if up.Str("product.price") != "49.99" || up.Str("product.cost") != "33" || up.Str("product.source_price") != "30" {
		t.Fatalf("refresh did not reprice: %s", up.Body)
	}

	// A locked manual price survives later imports; the cost still follows the source price.
	locked := c.Do("PUT", "/api/v1/products/"+pid+"/pricing", map[string]any{"price_locked": true, "price": "39.00", "compare_at_price": "45"}).Must(t, 200)
	if locked.Str("price") != "39" || locked.Get("price_locked") != true || locked.Get("pricing_rule_id") != nil || locked.Str("pricing.product.status") != "locked" ||
		locked.Str("compare_at_price") != "45" {
		t.Fatalf("lock wrong: %s", locked.Body)
	}
	again := ext.Do("POST", "/api/v1/imports/products", map[string]any{"payload": eurPayload("M1", "€40.00"), "decision": "update_existing", "target_product_id": pid}).Must(t, 201)
	if again.Str("product.price") != "39" || again.Str("product.cost") != "44" || again.Get("product.price_locked") != true {
		t.Fatalf("locked price changed by an import: %s", again.Body)
	}
	d2 := c.Do("GET", "/api/v1/products/"+pid, nil).Must(t, 200)
	if d2.Str("pricing.product.evaluation.price") != "65.99" || d2.Get("pricing.product.changed") != false {
		t.Fatalf("locked breakdown should show what the rules would give: %s", d2.Body)
	}
	unlocked := c.Do("PUT", "/api/v1/products/"+pid+"/pricing", map[string]any{"price_locked": false}).Must(t, 200)
	if unlocked.Str("price") != "65.99" || unlocked.Get("price_locked") != false || unlocked.Str("pricing_rule_id") != rule || unlocked.Str("compare_at_price") != "85.99" {
		t.Fatalf("unlock should reprice: %s", unlocked.Body)
	}
	c.Do("PUT", "/api/v1/products/"+pid+"/pricing", map[string]any{"price_locked": true, "price": "-1"}).Must(t, 422)

	// Without an exchange rate the cost stays in the source currency and no price is set.
	gbp := eurPayload("G1", "£15.00")
	gbp["url"] = "https://other.example.org/p/g1"
	nr := ext.Do("POST", "/api/v1/imports/products", map[string]any{"payload": gbp}).Must(t, 201)
	if nr.Get("product.price") != nil || nr.Str("product.cost") != "15" || nr.Str("product.cost_currency") != "GBP" ||
		!strings.Contains(strings.Join(toStrings(nr.Get("warnings")), "|"), "No exchange rate from GBP to USD") {
		t.Fatalf("missing rate not handled: %s", nr.Body)
	}
	if !strings.Contains(strings.Join(toStrings(nr.Get("import.warnings")), "|"), "No exchange rate") {
		t.Fatalf("warning not recorded with the import: %s", nr.Body)
	}

	// Audit trail of the pricing decisions.
	acts := map[string]bool{}
	for _, l := range c.Do("GET", "/api/v1/audit-logs?entity_type=product&entity_id="+pid, nil).Must(t, 200).Items() {
		acts[l["action"].(string)] = true
	}
	for _, a := range []string{"product.reprice", "product.price_lock", "product.price_unlock"} {
		if !acts[a] {
			t.Errorf("audit event %s missing", a)
		}
	}
}

func TestBulkRepriceDryRunAndApply(t *testing.T) {
	e := newEnv(t)
	c := e.owner
	a := c.Do("POST", "/api/v1/products", map[string]any{"title": "Alpha", "cost": "10", "cost_currency": "USD"}).Must(t, 201)
	if a.Str("price") != "10" || a.Get("price_locked") != false || a.Get("cost_manual") != true {
		t.Fatalf("default rule should price at cost: %s", a.Body)
	}
	b := c.Do("POST", "/api/v1/products", map[string]any{"title": "Bravo", "cost": "20", "cost_currency": "USD", "price": "25", "price_currency": "USD"}).Must(t, 201)
	if b.Get("price_locked") != true {
		t.Fatalf("an entered price is a locked manual price: %s", b.Body)
	}
	imp := c.Do("POST", "/api/v1/imports/products", map[string]any{"payload": productPayload("HB-1"), "source": "manual", "skip_supplier_link": true}).Must(t, 201)
	ch := imp.Str("product.id")
	if imp.Str("product.price") != "24.99" || imp.Str("product.source_price") != "24.99" {
		t.Fatalf("import with the default rule: %s", imp.Body)
	}
	setDefaultRule(t, c, map[string]any{"markup_percent": "100"})
	ids := []string{a.Str("id"), b.Str("id"), ch}

	viewer := e.userWithRole("viewer", "viewer@example.com")
	viewer.Do("POST", "/api/v1/products/reprice", map[string]any{"ids": ids, "dry_run": true}).Must(t, 403)
	c.Do("POST", "/api/v1/products/reprice", map[string]any{"ids": ids, "filter": map[string]any{}, "dry_run": true}).Must(t, 422)
	c.Do("POST", "/api/v1/products/reprice", map[string]any{"dry_run": true}).Must(t, 422)
	c.Do("POST", "/api/v1/products/reprice", map[string]any{"ids": []string{"00000000-0000-4000-8000-000000000999"}}).Must(t, 422)
	c.Do("POST", "/api/v1/products/reprice", map[string]any{"filter": map[string]any{"status": "bogus"}}).Must(t, 422)

	dry := c.Do("POST", "/api/v1/products/reprice", map[string]any{"filter": map[string]any{}, "dry_run": true}).Must(t, 200)
	if dry.Get("dry_run") != true || dry.Get("summary.products") != float64(3) || dry.Get("summary.changed") != float64(2) || dry.Get("summary.locked") != float64(1) ||
		dry.Get("summary.variants") != float64(2) || dry.Get("summary.variants_changed") != float64(2) {
		t.Fatalf("dry-run summary wrong: %s", dry.Body)
	}
	items := map[string]map[string]any{}
	for _, it := range dry.Items() {
		items[it["title"].(string)] = it
	}
	alpha := items["Alpha"]
	if alpha["before"].(map[string]any)["price"] != "10" || alpha["after"].(map[string]any)["price"] != "20" || alpha["changed"] != true || alpha["status"] != "repriced" {
		t.Fatalf("Alpha before/after wrong: %v", alpha)
	}
	if items["Bravo"]["status"] != "locked" || items["Bravo"]["changed"] != false || items["Bravo"]["after"].(map[string]any)["price"] != "25" {
		t.Fatalf("locked product must not change: %v", items["Bravo"])
	}
	bottle := items["Stainless Steel Water Bottle 750ml"]
	vs := bottle["variants"].([]any)
	if bottle["after"].(map[string]any)["price"] != "49.98" || len(vs) != 2 {
		t.Fatalf("variant product wrong: %v", bottle)
	}
	afterPrices := []string{vs[0].(map[string]any)["after"].(map[string]any)["price"].(string), vs[1].(map[string]any)["after"].(map[string]any)["price"].(string)}
	if strings.Join(afterPrices, ",") != "49.98,53.98" && strings.Join(afterPrices, ",") != "53.98,49.98" {
		t.Fatalf("variant after prices: %v", afterPrices)
	}
	// Nothing was saved by the dry run.
	if c.Do("GET", "/api/v1/products/"+a.Str("id"), nil).Must(t, 200).Str("price") != "10" {
		t.Fatal("dry run changed a product")
	}

	// A member with products.write can apply; locked prices stay.
	member := e.userWithRole("member", "member@example.com")
	applied := member.Do("POST", "/api/v1/products/reprice", map[string]any{"ids": ids, "dry_run": false}).Must(t, 200)
	if applied.Get("dry_run") != false || applied.Get("summary.changed") != float64(2) {
		t.Fatalf("apply wrong: %s", applied.Body)
	}
	if got := c.Do("GET", "/api/v1/products/"+a.Str("id"), nil).Must(t, 200); got.Str("price") != "20" || got.Get("pricing.product.changed") != false {
		t.Fatalf("Alpha not repriced: %s", got.Body)
	}
	if c.Do("GET", "/api/v1/products/"+b.Str("id"), nil).Must(t, 200).Str("price") != "25" {
		t.Fatal("locked price changed")
	}
	bd := c.Do("GET", "/api/v1/products/"+ch, nil).Must(t, 200)
	for _, v := range bd.Get("variants").([]any) {
		m := v.(map[string]any)
		if m["price"] != "49.98" && m["price"] != "53.98" {
			t.Fatalf("variant not repriced: %v", m)
		}
	}
	// Running it again changes nothing.
	if c.Do("POST", "/api/v1/products/reprice", map[string]any{"ids": ids}).Must(t, 200).Get("summary.changed") != float64(0) {
		t.Fatal("repricing twice must be idempotent")
	}
	logs := c.Do("GET", "/api/v1/audit-logs?action=product.reprice_bulk", nil).Must(t, 200)
	if len(logs.Items()) != 2 {
		t.Fatalf("bulk audit events: %s", logs.Body)
	}
}

func TestManualPricingSemantics(t *testing.T) {
	e := newEnv(t)
	c := e.owner
	setDefaultRule(t, c, map[string]any{"markup_percent": "25"})
	def := defaultRuleID(t, c)

	a := c.Do("POST", "/api/v1/products", map[string]any{"title": "Lamp", "cost": "10", "cost_currency": "USD"}).Must(t, 201)
	id := a.Str("id")
	if a.Str("price") != "12.5" || a.Get("price_locked") != false || a.Str("pricing_rule_id") != def || a.Str("pricing_rule_name") != "Default rule" {
		t.Fatalf("rule price on create: %s", a.Body)
	}
	base := map[string]any{"title": "Lamp", "cost": "20", "cost_currency": "USD", "price": "12.5", "price_currency": "USD"}
	// Changing the cost reprices an unlocked product.
	if got := c.Do("PUT", "/api/v1/products/"+id, base).Must(t, 200); got.Str("price") != "25" || got.Get("price_locked") != false {
		t.Fatalf("cost change should reprice: %s", got.Body)
	}
	// Entering a different price locks it.
	base["price"] = "30"
	if got := c.Do("PUT", "/api/v1/products/"+id, base).Must(t, 200); got.Str("price") != "30" || got.Get("price_locked") != true || got.Get("pricing_rule_id") != nil {
		t.Fatalf("manual price should lock: %s", got.Body)
	}
	// price_locked false hands the price back to the rules.
	base["price_locked"] = false
	if got := c.Do("PUT", "/api/v1/products/"+id, base).Must(t, 200); got.Str("price") != "25" || got.Get("price_locked") != false {
		t.Fatalf("unlocking via PUT should reprice: %s", got.Body)
	}
	c.Do("PUT", "/api/v1/products/"+id, map[string]any{"title": "Lamp", "price_locked": true}).Must(t, 422)

	// Variants: own cost, locked price, and inherited product cost.
	big := c.Do("POST", "/api/v1/products/"+id+"/variants", map[string]any{"title": "Big", "cost": "16", "cost_currency": "USD"}).Must(t, 201)
	if big.Str("price") != "20" || big.Get("price_locked") != false || big.Get("cost_manual") != true || big.Str("pricing_rule_id") != def {
		t.Fatalf("variant with cost: %s", big.Body)
	}
	small := c.Do("POST", "/api/v1/products/"+id+"/variants", map[string]any{"title": "Small", "price": "9", "currency": "USD"}).Must(t, 201)
	if small.Str("price") != "9" || small.Get("price_locked") != true {
		t.Fatalf("variant with a price: %s", small.Body)
	}
	mid := c.Do("POST", "/api/v1/products/"+id+"/variants", map[string]any{"title": "Mid"}).Must(t, 201)
	if mid.Str("price") != "25" || mid.Get("cost") != nil {
		t.Fatalf("variant inherits the product cost: %s", mid.Body)
	}
	upd := c.Do("PUT", "/api/v1/products/"+id+"/variants/"+big.Str("id"), map[string]any{"title": "Big", "cost": "24", "cost_currency": "USD",
		"price": "20", "currency": "USD"}).Must(t, 200)
	if upd.Str("price") != "30" {
		t.Fatalf("variant cost change should reprice: %s", upd.Body)
	}
	lk := c.Do("PUT", "/api/v1/products/"+id+"/variants/"+big.Str("id"), map[string]any{"title": "Big", "cost": "24", "cost_currency": "USD",
		"price": "31", "currency": "USD", "price_locked": true}).Must(t, 200)
	if lk.Str("price") != "31" || lk.Get("price_locked") != true {
		t.Fatalf("variant lock: %s", lk.Body)
	}
	detail := c.Do("GET", "/api/v1/products/"+id, nil).Must(t, 200)
	statuses := map[string]string{}
	for _, v := range detail.Get("pricing.variants").([]any) {
		m := v.(map[string]any)
		statuses[m["variant_id"].(string)] = m["plan"].(map[string]any)["status"].(string)
	}
	if statuses[big.Str("id")] != "locked" || statuses[small.Str("id")] != "locked" || statuses[mid.Str("id")] != "repriced" {
		t.Fatalf("variant plans wrong: %v", statuses)
	}

	// CSV: a source price is priced by the rules, a price column is kept as a manual price.
	csv := "Title,Source price,Source currency,Price,Currency\nCSV Rule Mug,8.00,USD,,\nCSV Manual Mug,8.00,USD,19.00,USD\n"
	c.Upload("/api/v1/import/csv/commit", "file", "p.csv", []byte(csv), map[string]string{"entity_type": "product"}).Must(t, 200)
	for _, it := range c.Do("GET", "/api/v1/products?q=CSV", nil).Must(t, 200).Items() {
		switch it["title"] {
		case "CSV Rule Mug":
			if it["price"] != "10" || it["source_price"] != "8" || it["price_locked"] != false {
				t.Errorf("csv rule mug: %v", it)
			}
		case "CSV Manual Mug":
			if it["price"] != "19" || it["price_locked"] != true {
				t.Errorf("csv manual mug: %v", it)
			}
		}
	}
}
