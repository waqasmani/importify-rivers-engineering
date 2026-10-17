package httpapi_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/testutil"
)

// orderFixture is a small catalog to order from: a manual supplier with a mug, an automation
// supplier with a T-shirt in variants, a lamp without a supplier record (grouped by its source
// domain), a draft product and an out-of-stock product.
type orderFixture struct {
	manualSupplier, autoSupplier string
	mug, tee, teeRedM, lamp      string
	draftProduct, soldOut        string
}

func (e *env) orderFixture() orderFixture {
	e.t.Helper()
	var f orderFixture
	f.manualSupplier = e.owner.Do("POST", "/api/v1/suppliers", map[string]any{"name": "Ningbo Mugs"}).Must(e.t, 201).Str("id")
	f.autoSupplier = e.owner.Do("POST", "/api/v1/suppliers", map[string]any{"name": "Tee Factory", "order_placement": "automation"}).Must(e.t, 201).Str("id")
	f.mug = e.publish("Enamel mug", "MUG-1", map[string]any{"supplier_id": f.manualSupplier, "source_url": "https://mugs.example.com/p/enamel",
		"cost": "3.10", "cost_currency": "EUR", "availability": "in_stock"})
	f.tee = e.publish("Cotton tee", "TEE", map[string]any{"supplier_id": f.autoSupplier, "source_url": "https://tees.example.com/p/cotton",
		"cost": "4", "cost_currency": "EUR", "availability": "in_stock"})
	f.teeRedM = e.owner.Do("POST", "/api/v1/products/"+f.tee+"/variants", map[string]any{"title": "Red / M", "sku": "TEE-RED-M",
		"options": map[string]string{"Color": "Red", "Size": "M"}, "price": "19.90", "currency": "EUR", "availability": "in_stock", "position": 0,
		"cost": "5.25", "cost_currency": "EUR"}).Must(e.t, 201).Str("id")
	f.lamp = e.publish("Desk lamp", "LAMP-1", map[string]any{"source_url": "https://lamps.example.org/item/7", "cost": "12", "cost_currency": "EUR",
		"availability": "in_stock"})
	f.draftProduct = e.owner.Do("POST", "/api/v1/products", map[string]any{"title": "Prototype vase", "sku": "VASE-1", "supplier_id": f.manualSupplier,
		"source_url": "https://mugs.example.com/p/vase", "cost": "8", "cost_currency": "EUR", "price": "20", "price_currency": "EUR"}).Must(e.t, 201).Str("id")
	f.soldOut = e.publish("Glass bottle", "BOTTLE-1", map[string]any{"supplier_id": f.manualSupplier, "source_url": "https://mugs.example.com/p/bottle",
		"cost": "2", "cost_currency": "EUR", "availability": "out_of_stock"})
	return f
}

var shipTo = map[string]any{"name": "Ada Lovelace", "line1": "12 Analytical Engine Way", "city": "London", "postal_code": "N1 7AA", "country": "GB",
	"phone": "+44 20 7946 0000"}

func orderBody(external string, lines ...map[string]any) map[string]any {
	return map[string]any{"external_id": external, "currency": "EUR", "customer": map[string]any{"name": "Ada Lovelace", "email": "ada.lovelace@example.org",
		"phone": "+44 20 7946 0000"}, "shipping_address": shipTo, "total": "99.00", "lines": lines}
}

func line(sku string, qty int, price string) map[string]any {
	return map[string]any{"sku": sku, "title": "Sold as " + sku, "quantity": qty, "unit_price": price}
}

// supplierOrders returns the order's supplier orders by supplier name (or source domain).
func supplierOrders(t *testing.T, c *testutil.Client, orderID string) map[string]*testutil.Response {
	t.Helper()
	out := map[string]*testutil.Response{}
	o := c.Do("GET", "/api/v1/orders/"+orderID, nil).Must(t, 200)
	for _, so := range o.Get("supplier_orders").([]any) {
		m := so.(map[string]any)
		d := c.Do("GET", "/api/v1/supplier-orders/"+m["id"].(string), nil).Must(t, 200)
		name := d.Str("supplier_name")
		if name == "" {
			name = d.Str("source_domain")
		}
		if m["status"] != "cancelled" {
			out[name] = d
		}
	}
	return out
}

func warningCodes(r *testutil.Response) []string {
	var out []string
	for _, w := range r.Get("warnings").([]any) {
		out = append(out, w.(map[string]any)["code"].(string))
	}
	return out
}

func hasCode(codes []string, code string) bool {
	for _, c := range codes {
		if c == code {
			return true
		}
	}
	return false
}

func (e *env) countRows(query string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.db.Pool.QueryRow(e.t.Context(), query, args...).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func confirmBody(r *testutil.Response, ack bool) map[string]any {
	return map[string]any{"content_hash": r.Str("content_hash"), "acknowledge_warnings": ack}
}

func TestOrderIntakeIsIdempotentAndPreparesDrafts(t *testing.T) {
	e := newEnv(t)
	f := e.orderFixture()
	shop, _, _ := e.integrationKey("Storefront", "orders.write")
	body := orderBody("1001", line("MUG-1", 2, "12.00"), line("TEE-RED-M", 1, "19.90"),
		map[string]any{"product_id": f.lamp, "title": "Lamp", "quantity": 1, "unit_price": "30"}, line("NOPE-404", 1, "5"))
	o := shop.Do("POST", "/api/v1/orders", body).Must(t, 201)
	id := o.Str("id")
	if !strings.HasPrefix(o.Str("number"), "SO-") || o.Str("channel") != "api" || o.Str("status") != "needs_attention" || o.Get("needs_attention") != true {
		t.Fatalf("order wrong: %s", o.Body)
	}
	res := map[string]string{}
	for _, l := range o.Get("lines").([]any) {
		m := l.(map[string]any)
		res[m["sku"].(string)+m["title"].(string)] = m["resolution"].(string)
		if m["sourcing"] != nil {
			t.Fatalf("integration keys must not see sourcing data: %v", m)
		}
	}
	if res["MUG-1Sold as MUG-1"] != "matched_product" || res["TEE-RED-MSold as TEE-RED-M"] != "matched_variant" || res["Lamp"] != "matched_product" ||
		res["NOPE-404Sold as NOPE-404"] != "unmatched" {
		t.Fatalf("resolution wrong: %v", res)
	}
	if n := len(o.Get("supplier_orders").([]any)); n != 3 {
		t.Fatalf("expected three drafts (two suppliers and one source domain), got %d: %s", n, o.Body)
	}
	if o.Get("supplier_orders.0.sourcing") != nil {
		t.Fatal("integration keys must not see supplier names or costs")
	}

	// The same submission again returns the same order (200) and creates nothing.
	again := shop.Do("POST", "/api/v1/orders", body).Must(t, 200)
	if again.Str("id") != id || e.countRows("SELECT count(*) FROM orders") != 1 || e.countRows("SELECT count(*) FROM supplier_orders") != 3 {
		t.Fatalf("repeat created another order: %s", again.Body)
	}
	changed := orderBody("1001", line("MUG-1", 3, "12.00"))
	if r := shop.Do("POST", "/api/v1/orders", changed).Must(t, 409); r.ErrorCode() != "order_conflict" || strings.Contains(string(r.Body), "Ada") {
		t.Fatalf("conflict wrong: %s", r.Body)
	}

	// People see the drafts with the sourcing snapshot.
	sos := supplierOrders(t, e.owner, id)
	mug, tee, lamp := sos["Ningbo Mugs"], sos["Tee Factory"], sos["lamps.example.org"]
	if mug == nil || tee == nil || lamp == nil {
		t.Fatalf("drafts not grouped by supplier and source domain: %v", sos)
	}
	if mug.Str("status") != "draft" || mug.Str("placement_method") != "manual" || mug.Str("items.0.sku") != "MUG-1" ||
		mug.Get("items.0.quantity") != float64(2) || mug.Str("items.0.expected_unit_cost") != "3.1" || mug.Str("expected_cost_total") != "6.2" ||
		mug.Str("items.0.source_url") != "https://mugs.example.com/p/enamel" || mug.Str("ship_to.line1") != "12 Analytical Engine Way" {
		t.Fatalf("manual draft wrong: %s", mug.Body)
	}
	if tee.Str("placement_method") != "automation" || tee.Str("items.0.variant_id") != f.teeRedM || tee.Str("items.0.options.Color") != "Red" ||
		tee.Str("items.0.expected_unit_cost") != "5.25" || tee.Str("items.0.sku") != "TEE-RED-M" {
		t.Fatalf("variant draft wrong: %s", tee.Body)
	}
	if lamp.Get("supplier_id") != nil || lamp.Str("source_domain") != "lamps.example.org" || lamp.Str("placement_method") != "manual" {
		t.Fatalf("domain draft wrong: %s", lamp.Body)
	}
	detail := e.owner.Do("GET", "/api/v1/orders/"+id, nil).Must(t, 200)
	if detail.Str("lines.0.sourcing.supplier_name") != "Ningbo Mugs" || detail.Str("customer.email") != "ada.lovelace@example.org" ||
		len(detail.Get("events").([]any)) == 0 {
		t.Fatalf("detail for people wrong: %s", detail.Body)
	}

	// Keys see only their own orders; lookups by external ID are per key.
	other, _, _ := e.integrationKey("ERP", "orders.write")
	other.Do("GET", "/api/v1/orders/"+id, nil).Must(t, 404)
	other.Do("GET", "/api/v1/orders/by-external-id/1001", nil).Must(t, 404)
	if l := other.Do("GET", "/api/v1/orders", nil).Must(t, 200); l.Get("total") != float64(0) {
		t.Fatalf("other key lists foreign orders: %s", l.Body)
	}
	shop.Do("GET", "/api/v1/orders/by-external-id/1001", nil).Must(t, 200)
	if l := shop.Do("GET", "/api/v1/orders?needs_attention=true", nil).Must(t, 200); l.Get("total") != float64(1) {
		t.Fatalf("own orders not listed: %s", l.Body)
	}
	// A different key may use the same external ID: it is another source system's order.
	dup := other.Do("POST", "/api/v1/orders", orderBody("1001", line("MUG-1", 1, "12.00"))).Must(t, 201)
	dsos := supplierOrders(t, e.owner, dup.Str("id"))
	if !hasCode(warningCodes(dsos["Ningbo Mugs"]), "possible_duplicate") {
		t.Fatalf("duplicate external ID not flagged: %s", dsos["Ningbo Mugs"].Body)
	}

	// Validation is strict.
	for name, bad := range map[string]map[string]any{
		"no external id": orderBody("", line("MUG-1", 1, "1")),
		"country":        {"external_id": "x1", "currency": "EUR", "shipping_address": map[string]any{"country": "XX"}, "lines": []any{line("MUG-1", 1, "1")}},
		"currency":       {"external_id": "x2", "currency": "EURO", "lines": []any{line("MUG-1", 1, "1")}},
		"quantity":       orderBody("x3", line("MUG-1", 0, "1")),
		"too many":       orderBody("x4", line("MUG-1", 10001, "1")),
		"price":          orderBody("x5", line("MUG-1", 1, "-1")),
		"no lines":       orderBody("x6"),
		"email":          {"external_id": "x7", "currency": "EUR", "customer": map[string]any{"email": "not-an-email"}, "lines": []any{line("MUG-1", 1, "1")}},
		"empty line":     orderBody("x8", map[string]any{"quantity": 1}),
	} {
		if r := shop.Do("POST", "/api/v1/orders", bad); r.Status != 422 {
			t.Errorf("%s: expected 422, got %d %s", name, r.Status, r.Body)
		}
	}
	many := []map[string]any{}
	for i := 0; i < 201; i++ {
		many = append(many, line("MUG-1", 1, "1"))
	}
	shop.Do("POST", "/api/v1/orders", orderBody("x9", many...)).Must(t, 422)
	shop.Do("POST", "/api/v1/orders", map[string]any{"external_id": "x10", "currency": "EUR", "lines": []any{line("MUG-1", 1, "1")}, "unknown": 1}).Must(t, 400)

	// People enter orders by hand; the manual channel is idempotent on external IDs too.
	member := e.userWithRole("member", "member@example.com")
	m := member.Do("POST", "/api/v1/orders", orderBody("PHONE-7", line("MUG-1", 1, "12"))).Must(t, 201)
	if m.Str("channel") != "manual" || m.Str("source") != "member user" {
		t.Fatalf("manual order wrong: %s", m.Body)
	}
	member.Do("POST", "/api/v1/orders", orderBody("PHONE-7", line("MUG-1", 1, "12"))).Must(t, 200)
	member.Do("POST", "/api/v1/orders", map[string]any{"currency": "EUR", "lines": []any{line("MUG-1", 1, "12")}}).Must(t, 201)
	member.Do("GET", "/api/v1/orders/by-external-id/PHONE-7", nil).Must(t, 200)
	if l := member.Do("GET", "/api/v1/orders?channel=manual", nil).Must(t, 200); l.Get("total") != float64(2) {
		t.Fatalf("channel filter wrong: %s", l.Body)
	}
	if l := member.Do("GET", "/api/v1/orders?q=PHONE", nil).Must(t, 200); l.Get("total") != float64(1) {
		t.Fatalf("search wrong: %s", l.Body)
	}
	member.Do("GET", "/api/v1/orders?status=bogus", nil).Must(t, 422)
	if l := member.Do("GET", "/api/v1/orders?from=2000-01-01T00:00:00Z&to=2001-01-01T00:00:00Z", nil).Must(t, 200); l.Get("total") != float64(0) {
		t.Fatalf("date filter wrong: %s", l.Body)
	}
	// Viewers cannot see customer data unless an administrator grants orders.read.
	viewer := e.userWithRole("viewer", "viewer@example.com")
	viewer.Do("GET", "/api/v1/orders", nil).Must(t, 403)
	viewer.Do("GET", "/api/v1/orders/"+id, nil).Must(t, 403)
	viewer.Do("GET", "/api/v1/supplier-orders", nil).Must(t, 403)
	viewer.Do("GET", "/api/v1/activity?entity_type=order&entity_id="+id, nil).Must(t, 403)
}

func TestSupplierOrderWarningsAndRemapRebuildOnlyDrafts(t *testing.T) {
	e := newEnv(t)
	f := e.orderFixture()
	member := e.userWithRole("member", "member@example.com")
	o := member.Do("POST", "/api/v1/orders", map[string]any{"external_id": "W-1", "currency": "EUR",
		"shipping_address": map[string]any{"name": "Ada", "line1": "1 Main St", "country": "DE"},
		"lines":            []any{line("MUG-1", 1, "1.00"), line("VASE-1", 1, "30"), line("BOTTLE-1", 1, "10"), line("TEE", 1, "25"), line("LOST", 1, "9")}}).Must(t, 201)
	id := o.Str("id")
	mug := supplierOrders(t, member, id)["Ningbo Mugs"]
	codes := warningCodes(mug)
	for _, want := range []string{"unmatched_lines", "negative_margin", "not_published", "availability", "incomplete_address"} {
		if !hasCode(codes, want) {
			t.Errorf("missing warning %s in %v", want, codes)
		}
	}
	tee := supplierOrders(t, member, id)["Tee Factory"]
	if !hasCode(warningCodes(tee), "variant_not_selected") {
		t.Errorf("product with variants matched without a variant should warn: %v", warningCodes(tee))
	}
	if q := member.Do("GET", "/api/v1/supplier-orders?queue=to_confirm", nil).Must(t, 200); q.Get("total") != float64(2) || q.Get("items.0.warning_count").(float64) < 1 {
		t.Fatalf("queue wrong: %s", q.Body)
	}

	// Map the unmatched line to the lamp: it gets its own draft and the order no longer needs attention.
	lines := o.Get("lines").([]any)
	lost := lines[4].(map[string]any)["id"].(string)
	teeLine := lines[3].(map[string]any)["id"].(string)
	member.Do("PUT", "/api/v1/orders/"+id+"/lines/"+lost, map[string]any{"product_id": f.lamp, "variant_id": nil, "quantity": 2}).Must(t, 200)
	member.Do("PUT", "/api/v1/orders/"+id+"/lines/"+lost, map[string]any{"product_id": f.lamp, "variant_id": f.teeRedM, "quantity": 2}).Must(t, 422)
	member.Do("PUT", "/api/v1/orders/"+id+"/lines/"+lost, map[string]any{"product_id": f.lamp, "variant_id": nil, "quantity": 0}).Must(t, 422)
	after := member.Do("GET", "/api/v1/orders/"+id, nil).Must(t, 200)
	if after.Str("status") != "awaiting_confirmation" || after.Get("needs_attention") != false || after.Str("lines.4.resolution") != "manual" {
		t.Fatalf("re-map did not resolve the order: %s", after.Body)
	}
	if hasCode(warningCodes(supplierOrders(t, member, id)["Ningbo Mugs"]), "unmatched_lines") {
		t.Fatal("unmatched warning remains after mapping")
	}

	// Confirm the mug order; later changes to other lines leave it exactly as confirmed.
	mug = supplierOrders(t, member, id)["Ningbo Mugs"]
	confirmed := e.owner.Do("POST", "/api/v1/supplier-orders/"+mug.Str("id")+"/confirm", confirmBody(mug, true)).Must(t, 200)
	if confirmed.Str("status") != "confirmed" || confirmed.Str("content_hash") != mug.Str("content_hash") || confirmed.Str("confirmed_by") != "Olive Owner" {
		t.Fatalf("confirm wrong: %s", confirmed.Body)
	}
	member.Do("PUT", "/api/v1/orders/"+id+"/lines/"+teeLine, map[string]any{"product_id": f.tee, "variant_id": f.teeRedM, "quantity": 3}).Must(t, 200)
	member.Do("POST", "/api/v1/orders/"+id+"/prepare", nil).Must(t, 200)
	still := member.Do("GET", "/api/v1/supplier-orders/"+mug.Str("id"), nil).Must(t, 200)
	if still.Str("status") != "confirmed" || still.Str("content_hash") != mug.Str("content_hash") || string(mustJSON(still.Get("items"))) != string(mustJSON(mug.Get("items"))) {
		t.Fatalf("a confirmed supplier order changed: %s", still.Body)
	}
	tee = supplierOrders(t, member, id)["Tee Factory"]
	if tee.Get("items.0.quantity") != float64(3) || tee.Str("items.0.variant_id") != f.teeRedM || hasCode(warningCodes(tee), "variant_not_selected") {
		t.Fatalf("draft not rebuilt: %s", tee.Body)
	}
	// Lines of confirmed supplier orders cannot change.
	mugLine := lines[0].(map[string]any)["id"].(string)
	if r := member.Do("PUT", "/api/v1/orders/"+id+"/lines/"+mugLine, map[string]any{"product_id": f.mug, "variant_id": nil, "quantity": 5}).Must(t, 409); r.ErrorCode() != "supplier_order_locked" {
		t.Fatalf("locked line edited: %s", r.Body)
	}
	member.Do("POST", "/api/v1/orders/"+id+"/lines/"+mugLine+"/cancel", nil).Must(t, 409)
	// Editing the ship-to of a draft changes its hash; a confirmed one cannot be edited.
	member.Do("PUT", "/api/v1/supplier-orders/"+mug.Str("id"), map[string]any{"ship_to": shipTo, "note_to_supplier": ""}).Must(t, 409)
	edited := member.Do("PUT", "/api/v1/supplier-orders/"+tee.Str("id"), map[string]any{"ship_to": shipTo, "note_to_supplier": "No invoice in the parcel"}).Must(t, 200)
	if edited.Str("content_hash") == tee.Str("content_hash") || edited.Str("ship_to.city") != "London" {
		t.Fatalf("draft edit wrong: %s", edited.Body)
	}
	member.Do("PUT", "/api/v1/supplier-orders/"+tee.Str("id"), map[string]any{"ship_to": map[string]any{"country": "Germany"}, "note_to_supplier": ""}).Must(t, 422)
	// Cancelling a line removes it from its draft.
	vase := lines[1].(map[string]any)["id"].(string)
	member.Do("POST", "/api/v1/orders/"+id+"/lines/"+vase+"/cancel", nil).Must(t, 409) // in the confirmed mug order
	lamp := supplierOrders(t, member, id)["lamps.example.org"]
	member.Do("POST", "/api/v1/orders/"+id+"/lines/"+lost+"/cancel", nil).Must(t, 200)
	member.Do("GET", "/api/v1/supplier-orders/"+lamp.Str("id"), nil).Must(t, 404) // the empty draft is gone
}

// findEvent returns the first request the receiver got for an event type.
func findEvent(rc *receiver, typ string) *receivedRequest {
	for _, r := range rc.requests() {
		if r.header.Get("X-REA-Event-Type") == typ {
			return &r
		}
	}
	return nil
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func TestConfirmationGate(t *testing.T) {
	e := newEnvWith(t, localWebhooks)
	f := e.orderFixture()
	rc := newReceiver(t)
	e.owner.Do("POST", "/api/v1/integrations/webhooks", map[string]any{"name": "Automation", "url": rc.srv.URL, "events": []string{"supplier_order.confirmed"},
		"enabled": true}).Must(t, 201)
	shop, _, shopToken := e.integrationKey("Storefront", "orders.write", "catalog.read")
	report, _, _ := e.integrationKey("Buyer bot", "supplier_orders.report")
	allScopes, _, _ := e.integrationKey("Everything", "orders.write", "supplier_orders.report", "catalog.read", "imports.write")
	o := shop.Do("POST", "/api/v1/orders", orderBody("G-1", line("TEE-RED-M", 2, "19.90"), line("MUG-1", 1, "12"))).Must(t, 201)
	sos := supplierOrders(t, e.owner, o.Str("id"))
	tee, mug := sos["Tee Factory"], sos["Ningbo Mugs"]
	teePath := "/api/v1/supplier-orders/" + tee.Str("id")
	_ = f

	// Nobody but a signed-in person with orders.place can confirm.
	for _, c := range []*testutil.Client{shop, report, allScopes} {
		if r := c.Do("POST", teePath+"/confirm", confirmBody(tee, true)); r.Status != 403 {
			t.Errorf("integration key confirmed: %d %s", r.Status, r.Body)
		}
	}
	pat := e.owner.Do("POST", "/api/v1/auth/tokens", map[string]any{"name": "script", "expires_in_days": 7,
		"scopes": []string{"orders.read", "orders.write", "orders.place"}}).Must(t, 201)
	patClient := e.client()
	patClient.Bearer = pat.Str("token")
	patClient.Do("POST", teePath+"/confirm", confirmBody(tee, true)).Must(t, 403)
	ext, _ := e.pairExtension()
	ext.Do("POST", teePath+"/confirm", confirmBody(tee, true)).Must(t, 403)
	member := e.userWithRole("member", "member@example.com")
	member.Do("POST", teePath+"/confirm", confirmBody(tee, true)).Must(t, 403)
	noCSRF := e.login("owner@example.com", ownerPassword)
	noCSRF.CSRF = ""
	if r := noCSRF.Do("POST", teePath+"/confirm", confirmBody(tee, true)).Must(t, 403); r.ErrorCode() != "csrf_failed" {
		t.Fatalf("CSRF not enforced: %s", r.Body)
	}
	_ = shopToken

	// Placement before confirmation is refused for every caller type.
	placed := map[string]any{"supplier_order_number": "X-1"}
	if r := e.owner.Do("POST", teePath+"/placed", placed).Must(t, 409); r.ErrorCode() != "not_confirmed" {
		t.Fatalf("session placed a draft: %s", r.Body)
	}
	patClient.Do("POST", teePath+"/placed", placed).Must(t, 409)
	member.Do("POST", "/api/v1/supplier-orders/"+mug.Str("id")+"/placed", placed).Must(t, 409)
	report.Do("POST", teePath+"/placed", placed).Must(t, 409)
	allScopes.Do("POST", teePath+"/placed", placed).Must(t, 409)
	shop.Do("POST", teePath+"/placed", placed).Must(t, 403)
	ext.Do("POST", teePath+"/placed", placed).Must(t, 403)
	report.Do("POST", "/api/v1/supplier-orders/"+mug.Str("id")+"/placed", placed).Must(t, 404) // manual supplier: not the automation's business
	// Drafts are never readable by the automation.
	report.Do("GET", teePath, nil).Must(t, 404)
	if l := report.Do("GET", "/api/v1/supplier-orders", nil).Must(t, 200); l.Get("total") != float64(0) {
		t.Fatalf("automation sees drafts: %s", l.Body)
	}
	e.worker.DrainWebhooks(t.Context())
	countConfirmed := func() int {
		return e.countRows("SELECT count(*) FROM webhook_deliveries WHERE event_type = 'supplier_order.confirmed' AND supplier_order_id = $1", tee.Str("id"))
	}
	if countConfirmed() != 0 || len(rc.requests()) != 0 {
		t.Fatal("a supplier_order.confirmed delivery exists before confirmation")
	}

	// A wrong hash, missing acknowledgement or a repeat are refused.
	if r := e.owner.Do("POST", teePath+"/confirm", map[string]any{"content_hash": strings.Repeat("a", 64), "acknowledge_warnings": true}).Must(t, 409); r.ErrorCode() != "content_changed" {
		t.Fatalf("wrong hash: %s", r.Body)
	}
	e.owner.Do("POST", teePath+"/confirm", map[string]any{"content_hash": "short", "acknowledge_warnings": true}).Must(t, 422)
	// Changing the draft after it was reviewed invalidates the reviewed hash.
	e.owner.Do("PUT", teePath, map[string]any{"ship_to": shipTo, "note_to_supplier": "Gift wrap"}).Must(t, 200)
	if r := e.owner.Do("POST", teePath+"/confirm", confirmBody(tee, true)).Must(t, 409); r.ErrorCode() != "content_changed" {
		t.Fatalf("stale hash accepted: %s", r.Body)
	}
	tee = e.owner.Do("GET", teePath, nil).Must(t, 200)
	// Out of stock makes a warning that must be acknowledged.
	e.owner.Do("PUT", "/api/v1/products/"+f.tee+"/variants/"+f.teeRedM, map[string]any{"title": "Red / M", "sku": "TEE-RED-M",
		"options": map[string]string{"Color": "Red", "Size": "M"}, "price": "19.90", "currency": "EUR", "availability": "out_of_stock", "position": 0,
		"cost": "5.25", "cost_currency": "EUR"}).Must(t, 200)
	if r := e.owner.Do("POST", teePath+"/confirm", confirmBody(tee, true)).Must(t, 409); r.ErrorCode() != "content_changed" {
		t.Fatalf("a new warning must invalidate the reviewed hash: %s", r.Body)
	}
	tee = e.owner.Do("GET", teePath, nil).Must(t, 200)
	if !hasCode(warningCodes(tee), "availability") {
		t.Fatalf("availability warning missing: %s", tee.Body)
	}
	if r := e.owner.Do("POST", teePath+"/confirm", confirmBody(tee, false)).Must(t, 422); r.ErrorCode() != "warnings_not_acknowledged" {
		t.Fatalf("warnings not enforced: %s", r.Body)
	}
	if countConfirmed() != 0 {
		t.Fatal("a refused confirmation created a delivery")
	}
	ok := e.owner.Do("POST", teePath+"/confirm", confirmBody(tee, true)).Must(t, 200)
	if ok.Str("status") != "confirmed" || ok.Get("confirmed_at") == nil || ok.Get("attempt") != float64(1) {
		t.Fatalf("confirmation wrong: %s", ok.Body)
	}
	if r := e.owner.Do("POST", teePath+"/confirm", confirmBody(tee, true)).Must(t, 409); r.ErrorCode() != "not_draft" {
		t.Fatalf("confirmed twice: %s", r.Body)
	}
	if countConfirmed() != 1 {
		t.Fatalf("expected exactly one supplier_order.confirmed delivery, got %d", countConfirmed())
	}
	e.waitFor("the confirmed event", func() bool { return len(rc.requests()) == 1 })
	var ev map[string]any
	if err := json.Unmarshal(rc.requests()[0].body, &ev); err != nil {
		t.Fatal(err)
	}
	so := ev["supplier_order"].(map[string]any)
	if ev["type"] != "supplier_order.confirmed" || so["id"] != tee.Str("id") || so["ship_to"].(map[string]any)["line1"] != "12 Analytical Engine Way" ||
		so["items"].([]any)[0].(map[string]any)["source_url"] != "https://tees.example.com/p/cotton" || so["costs"] != nil ||
		so["items"].([]any)[0].(map[string]any)["cost"] != nil || so["note_to_supplier"] != "Gift wrap" {
		t.Fatalf("confirmed payload wrong: %s", rc.requests()[0].body)
	}
	// Confirming a manual order hands nothing to the automation.
	mug = e.owner.Do("GET", "/api/v1/supplier-orders/"+mug.Str("id"), nil).Must(t, 200)
	e.owner.Do("POST", "/api/v1/supplier-orders/"+mug.Str("id")+"/confirm", confirmBody(mug, true)).Must(t, 200)
	if e.countRows("SELECT count(*) FROM webhook_deliveries WHERE event_type = 'supplier_order.confirmed'") != 1 {
		t.Fatal("a manual supplier order was handed to the automation")
	}
	audit := e.owner.Do("GET", "/api/v1/audit-logs?action=supplier_order.confirm", nil).Must(t, 200).Items()
	if len(audit) != 2 || audit[0]["actor_label"] != "owner@example.com" {
		t.Fatalf("confirmation not audited: %v", audit)
	}
	if o := e.owner.Do("GET", "/api/v1/orders/"+o.Str("id"), nil).Must(t, 200); o.Str("status") != "in_progress" {
		t.Fatalf("status after confirming: %s", o.Str("status"))
	}

	// An order without a ship-to cannot be confirmed.
	bare := e.owner.Do("POST", "/api/v1/orders", map[string]any{"currency": "EUR", "lines": []any{line("MUG-1", 1, "12")}}).Must(t, 201)
	bareSO := supplierOrders(t, e.owner, bare.Str("id"))["Ningbo Mugs"]
	if r := e.owner.Do("POST", "/api/v1/supplier-orders/"+bareSO.Str("id")+"/confirm", confirmBody(bareSO, true)).Must(t, 422); r.ErrorCode() != "ship_to_missing" {
		t.Fatalf("missing ship-to accepted: %s", r.Body)
	}
}

func TestAutomationReportsPlacementFailureAndShipment(t *testing.T) {
	e := newEnvWith(t, localWebhooks)
	e.orderFixture()
	rc := newReceiver(t)
	created := e.owner.Do("POST", "/api/v1/integrations/webhooks", map[string]any{"name": "Automation", "url": rc.srv.URL,
		"events":  []string{"supplier_order.confirmed", "supplier_order.failed", "supplier_order.placed", "supplier_order.shipped", "order.status_changed"},
		"enabled": true, "include_costs": true}).Must(t, 201)
	secret := created.Str("secret")
	shop, _, _ := e.integrationKey("Storefront", "orders.write")
	report, _, _ := e.integrationKey("Buyer bot", "supplier_orders.report")
	o := shop.Do("POST", "/api/v1/orders", orderBody("A-1", line("TEE-RED-M", 1, "19.90"))).Must(t, 201)
	id := o.Str("id")
	tee := supplierOrders(t, e.owner, id)["Tee Factory"]
	path := "/api/v1/supplier-orders/" + tee.Str("id")
	e.owner.Do("POST", path+"/confirm", confirmBody(tee, true)).Must(t, 200)
	e.waitFor("the confirmed event", func() bool { return findEvent(rc, "supplier_order.confirmed") != nil })
	first := findEvent(rc, "supplier_order.confirmed")
	verifyIndependently(t, secret, *first)
	var firstEv map[string]any
	_ = json.Unmarshal(first.body, &firstEv)
	fso := firstEv["supplier_order"].(map[string]any)
	if fso["items"].([]any)[0].(map[string]any)["cost"].(map[string]any)["expected_unit_cost"] != "5.25" ||
		fso["costs"].(map[string]any)["expected_cost_total"] != "5.25" || firstEv["order"].(map[string]any)["status"] != "in_progress" {
		t.Fatalf("include_costs endpoint without costs, or a stale order status: %s", first.body)
	}

	// The automation reads what it must place.
	if l := report.Do("GET", "/api/v1/supplier-orders?queue=to_place", nil).Must(t, 200); l.Get("total") != float64(1) || l.Str("items.0.id") != tee.Str("id") {
		t.Fatalf("automation queue wrong: %s", l.Body)
	}
	if d := report.Do("GET", path, nil).Must(t, 200); d.Str("ship_to.name") != "Ada Lovelace" || d.Str("items.0.options.Size") != "M" {
		t.Fatalf("automation detail wrong: %s", d.Body)
	}
	report.Do("GET", "/api/v1/orders", nil).Must(t, 403)
	report.Do("POST", "/api/v1/orders", orderBody("R-1", line("MUG-1", 1, "1"))).Must(t, 403)
	shop.Do("POST", path+"/failed", map[string]any{"reason": "x"}).Must(t, 403)
	shop.Do("GET", path, nil).Must(t, 403)

	// Failure, then retry: the same supplier order is handed off again.
	report.Do("POST", path+"/failed", map[string]any{"reason": ""}).Must(t, 422)
	failed := report.Do("POST", path+"/failed", map[string]any{"reason": "Supplier site rejected the card"}).Must(t, 200)
	if failed.Str("status") != "failed" || failed.Str("failure_reason") != "Supplier site rejected the card" {
		t.Fatalf("failure wrong: %s", failed.Body)
	}
	if od := e.owner.Do("GET", "/api/v1/orders/"+id, nil).Must(t, 200); od.Get("needs_attention") != true {
		t.Fatalf("failed order does not need attention: %s", od.Body)
	}
	if q := e.owner.Do("GET", "/api/v1/supplier-orders/queues", nil).Must(t, 200); q.Get("failed") != float64(1) {
		t.Fatalf("queues wrong: %s", q.Body)
	}
	report.Do("POST", path+"/placed", map[string]any{"supplier_order_number": "T-1"}).Must(t, 409)
	report.Do("POST", path+"/retry", nil).Must(t, 403)
	member := e.userWithRole("member", "member@example.com")
	member.Do("POST", path+"/retry", nil).Must(t, 403)
	retried := e.owner.Do("POST", path+"/retry", nil).Must(t, 200)
	if retried.Str("status") != "confirmed" || retried.Get("attempt") != float64(2) {
		t.Fatalf("retry wrong: %s", retried.Body)
	}
	if n := e.countRows("SELECT count(*) FROM webhook_deliveries WHERE event_type = 'supplier_order.confirmed' AND supplier_order_id = $1", tee.Str("id")); n != 2 {
		t.Fatalf("retry should hand off again: %d confirmed deliveries", n)
	}

	// Placement and shipment reports.
	report.Do("POST", path+"/placed", map[string]any{"supplier_order_number": ""}).Must(t, 422)
	report.Do("POST", path+"/placed", map[string]any{"supplier_order_number": "T-1", "actual_cost": "5.10"}).Must(t, 422)
	pl := report.Do("POST", path+"/placed", map[string]any{"supplier_order_number": "T-1001", "actual_cost": "5.10", "currency": "EUR"}).Must(t, 200)
	if pl.Str("status") != "placed" || !strings.HasPrefix(pl.Str("placed_by"), "integration key Buyer bot") || pl.Str("actual_cost") != "5.1" {
		t.Fatalf("placed wrong: %s", pl.Body)
	}
	report.Do("POST", path+"/placed", map[string]any{"supplier_order_number": "T-1001"}).Must(t, 409)
	report.Do("POST", path+"/delivered", nil).Must(t, 409)
	report.Do("POST", path+"/shipment", map[string]any{"carrier": "DHL", "tracking_number": "JD0001", "tracking_url": "http://track.example.com/JD0001"}).Must(t, 422)
	report.Do("POST", path+"/shipment", map[string]any{"carrier": "DHL", "tracking_number": "JD0001", "tracking_url": "javascript:alert(1)"}).Must(t, 422)
	sh := report.Do("POST", path+"/shipment", map[string]any{"carrier": "DHL", "tracking_number": "JD0001", "tracking_url": "https://track.example.com/JD0001"}).Must(t, 200)
	if sh.Str("status") != "shipped" || sh.Str("tracking_url") != "https://track.example.com/JD0001" {
		t.Fatalf("shipment wrong: %s", sh.Body)
	}
	if od := shop.Do("GET", "/api/v1/orders/"+id, nil).Must(t, 200); od.Str("status") != "shipped" || od.Str("supplier_orders.0.tracking_number") != "JD0001" {
		t.Fatalf("storefront does not see tracking: %s", od.Body)
	}
	report.Do("POST", path+"/delivered", nil).Must(t, 200)
	if od := shop.Do("GET", "/api/v1/orders/"+id, nil).Must(t, 200); od.Str("status") != "delivered" {
		t.Fatalf("not delivered: %s", od.Body)
	}
	e.waitFor("all events", func() bool { return len(rc.requests()) >= 9 })
	types := map[string]int{}
	for _, r := range rc.requests() {
		var ev map[string]any
		_ = json.Unmarshal(r.body, &ev)
		types[ev["type"].(string)]++
		verifyIndependently(t, secret, r)
	}
	if types["supplier_order.confirmed"] != 2 || types["supplier_order.failed"] != 1 || types["supplier_order.placed"] != 1 || types["supplier_order.shipped"] != 1 ||
		types["order.status_changed"] != 4 {
		t.Fatalf("events wrong: %v", types)
	}

	// A failed automated order can be handed to a person instead.
	o2 := shop.Do("POST", "/api/v1/orders", orderBody("A-2", line("TEE-RED-M", 1, "19.90"))).Must(t, 201)
	tee2 := supplierOrders(t, e.owner, o2.Str("id"))["Tee Factory"]
	p2 := "/api/v1/supplier-orders/" + tee2.Str("id")
	e.owner.Do("POST", p2+"/confirm", confirmBody(tee2, true)).Must(t, 200)
	report.Do("POST", p2+"/failed", map[string]any{"reason": "Captcha"}).Must(t, 200)
	sw := member.Do("POST", p2+"/switch-to-manual", nil).Must(t, 200)
	if sw.Str("status") != "confirmed" || sw.Str("placement_method") != "manual" {
		t.Fatalf("switch wrong: %s", sw.Body)
	}
	report.Do("GET", p2, nil).Must(t, 404)
	report.Do("POST", p2+"/placed", map[string]any{"supplier_order_number": "M-1"}).Must(t, 404)
	member.Do("POST", p2+"/placed", map[string]any{"supplier_order_number": "M-1", "note": "Ordered by phone"}).Must(t, 200)
}

func TestCancelAndRevertRules(t *testing.T) {
	e := newEnvWith(t, localWebhooks)
	e.orderFixture()
	rc := newReceiver(t)
	e.owner.Do("POST", "/api/v1/integrations/webhooks", map[string]any{"name": "Automation", "url": rc.srv.URL,
		"events": []string{"supplier_order.cancelled", "supplier_order.confirmed"}, "enabled": true}).Must(t, 201)
	shop, _, _ := e.integrationKey("Storefront", "orders.write")
	member := e.userWithRole("member", "member@example.com")
	o := shop.Do("POST", "/api/v1/orders", orderBody("C-1", line("TEE-RED-M", 1, "19.90"), line("MUG-1", 1, "12"))).Must(t, 201)
	id := o.Str("id")
	sos := supplierOrders(t, e.owner, id)
	tee, mug := sos["Tee Factory"], sos["Ningbo Mugs"]
	teePath, mugPath := "/api/v1/supplier-orders/"+tee.Str("id"), "/api/v1/supplier-orders/"+mug.Str("id")

	// Revert of a confirmed order needs orders.place and voids the confirmation; automation is told.
	e.owner.Do("POST", teePath+"/confirm", confirmBody(tee, true)).Must(t, 200)
	member.Do("POST", teePath+"/revert", nil).Must(t, 403)
	rev := e.owner.Do("POST", teePath+"/revert", nil).Must(t, 200)
	if rev.Str("status") != "draft" || rev.Get("confirmed_at") != nil || rev.Str("content_hash") != tee.Str("content_hash") {
		t.Fatalf("revert wrong: %s", rev.Body)
	}
	if n := e.countRows("SELECT count(*) FROM webhook_deliveries WHERE event_type = 'supplier_order.cancelled' AND supplier_order_id = $1", tee.Str("id")); n != 1 {
		t.Fatalf("automation not told about the withdrawn confirmation: %d", n)
	}
	e.owner.Do("POST", teePath+"/revert", nil).Must(t, 409)

	// Cancelling a draft gives its lines back; preparing again makes a new draft.
	member.Do("POST", mugPath+"/cancel", map[string]any{}).Must(t, 200)
	if od := member.Do("GET", "/api/v1/orders/"+id, nil).Must(t, 200); od.Str("status") != "needs_attention" {
		t.Fatalf("released lines should need attention: %s", od.Body)
	}
	member.Do("POST", "/api/v1/orders/"+id+"/prepare", nil).Must(t, 200)
	mug = supplierOrders(t, member, id)["Ningbo Mugs"]
	if mug.Str("id") == "" || mug.Str("id") == strings.TrimPrefix(mugPath, "/api/v1/supplier-orders/") {
		t.Fatal("prepare did not create a new draft")
	}
	mugPath = "/api/v1/supplier-orders/" + mug.Str("id")
	if n := e.countRows("SELECT count(*) FROM webhook_deliveries WHERE event_type = 'supplier_order.cancelled'"); n != 1 {
		t.Fatal("cancelling a draft must not send an event: it was never handed off")
	}

	// A placed order can only be marked cancelled with a reason; the customer order then cannot be cancelled.
	e.owner.Do("POST", mugPath+"/confirm", confirmBody(mug, true)).Must(t, 200)
	member.Do("POST", mugPath+"/placed", map[string]any{"supplier_order_number": "NB-55"}).Must(t, 200)
	if r := shop.Do("POST", "/api/v1/orders/"+id+"/cancel", map[string]any{"reason": "Customer changed their mind"}).Must(t, 409); r.ErrorCode() != "supplier_order_placed" {
		t.Fatalf("order with a placed supplier order cancelled: %s", r.Body)
	}
	member.Do("POST", mugPath+"/cancel", map[string]any{}).Must(t, 422)
	member.Do("POST", mugPath+"/cancel", map[string]any{"reason": "Supplier cancelled: out of stock"}).Must(t, 200)
	member.Do("POST", mugPath+"/cancel", map[string]any{"reason": "again"}).Must(t, 409)

	// Now nothing is placed: the storefront may cancel the customer order.
	cancelled := shop.Do("POST", "/api/v1/orders/"+id+"/cancel", map[string]any{"reason": "Customer changed their mind"}).Must(t, 200)
	if cancelled.Str("status") != "cancelled" || cancelled.Get("cancelled_at") == nil {
		t.Fatalf("cancel wrong: %s", cancelled.Body)
	}
	for _, so := range cancelled.Get("supplier_orders").([]any) {
		if so.(map[string]any)["status"] != "cancelled" {
			t.Fatalf("supplier order left open: %s", cancelled.Body)
		}
	}
	shop.Do("POST", "/api/v1/orders/"+id+"/cancel", nil).Must(t, 409)
	member.Do("POST", "/api/v1/orders/"+id+"/prepare", nil).Must(t, 409)
	other, _, _ := e.integrationKey("ERP", "orders.write")
	other.Do("POST", "/api/v1/orders/"+id+"/cancel", nil).Must(t, 404)
	// Keys never reach the people-only endpoints.
	shop.Do("POST", "/api/v1/orders/"+id+"/prepare", nil).Must(t, 403)
	shop.Do("POST", "/api/v1/orders/"+id+"/erase-customer", nil).Must(t, 403)
	shop.Do("POST", teePath+"/revert", nil).Must(t, 403)
	shop.Do("PUT", teePath, map[string]any{"ship_to": shipTo, "note_to_supplier": ""}).Must(t, 403)
}

func TestRevertedDraftMergesWithANewerDraftOfTheSameSupplier(t *testing.T) {
	e := newEnv(t)
	f := e.orderFixture()
	o := e.owner.Do("POST", "/api/v1/orders", orderBody("M-1", line("MUG-1", 1, "12"), line("LAMP-1", 1, "40"))).Must(t, 201)
	id := o.Str("id")
	a := supplierOrders(t, e.owner, id)["Ningbo Mugs"]
	e.owner.Do("POST", "/api/v1/supplier-orders/"+a.Str("id")+"/confirm", confirmBody(a, true)).Must(t, 200)
	// The lamp line is re-mapped to another product of the same supplier while A is confirmed: a new draft B.
	lampLine := o.Get("lines").([]any)[1].(map[string]any)["id"].(string)
	e.owner.Do("PUT", "/api/v1/orders/"+id+"/lines/"+lampLine, map[string]any{"product_id": f.soldOut, "variant_id": nil, "quantity": 1}).Must(t, 200)
	var b string
	for _, so := range e.owner.Do("GET", "/api/v1/orders/"+id, nil).Must(t, 200).Get("supplier_orders").([]any) {
		m := so.(map[string]any)
		if m["status"] == "draft" {
			b = m["id"].(string)
		}
	}
	if b == "" || b == a.Str("id") {
		t.Fatal("expected a second draft for the same supplier")
	}
	// Reverting A leaves two drafts of one supplier until they are prepared again; then they merge into A.
	if r := e.owner.Do("POST", "/api/v1/supplier-orders/"+a.Str("id")+"/revert", nil).Must(t, 200); r.Str("status") != "draft" || len(r.Get("items").([]any)) != 1 {
		t.Fatalf("revert wrong: %s", r.Body)
	}
	e.owner.Do("POST", "/api/v1/orders/"+id+"/prepare", nil).Must(t, 200)
	merged := e.owner.Do("GET", "/api/v1/supplier-orders/"+a.Str("id"), nil).Must(t, 200)
	if merged.Str("status") != "draft" || len(merged.Get("items").([]any)) != 2 {
		t.Fatalf("drafts not merged into the older one: %s", merged.Body)
	}
	e.owner.Do("GET", "/api/v1/supplier-orders/"+b, nil).Must(t, 404)
}

func TestOrderStatusFollowsSupplierOrders(t *testing.T) {
	e := newEnv(t)
	e.orderFixture()
	member := e.userWithRole("member", "member@example.com")
	o := member.Do("POST", "/api/v1/orders", orderBody("S-1", line("MUG-1", 1, "12"), line("LAMP-1", 1, "40"))).Must(t, 201)
	id := o.Str("id")
	status := func() string { return member.Do("GET", "/api/v1/orders/"+id, nil).Must(t, 200).Str("status") }
	if status() != "awaiting_confirmation" {
		t.Fatalf("expected awaiting_confirmation, got %s", status())
	}
	sos := supplierOrders(t, member, id)
	mug, lamp := sos["Ningbo Mugs"], sos["lamps.example.org"]
	for _, so := range []*testutil.Response{mug, lamp} {
		e.owner.Do("POST", "/api/v1/supplier-orders/"+so.Str("id")+"/confirm", confirmBody(so, true)).Must(t, 200)
	}
	if status() != "in_progress" {
		t.Fatalf("expected in_progress, got %s", status())
	}
	steps := []struct {
		so, action string
		body       map[string]any
		want       string
	}{
		{mug.Str("id"), "placed", map[string]any{"supplier_order_number": "A"}, "in_progress"},
		{lamp.Str("id"), "placed", map[string]any{"supplier_order_number": "B"}, "placed"},
		{mug.Str("id"), "shipment", map[string]any{"tracking_number": "T1"}, "partially_shipped"},
		{lamp.Str("id"), "shipment", map[string]any{"tracking_number": "T2", "carrier": "UPS"}, "shipped"},
		{mug.Str("id"), "delivered", nil, "shipped"},
		{lamp.Str("id"), "delivered", nil, "delivered"},
	}
	for _, st := range steps {
		member.Do("POST", "/api/v1/supplier-orders/"+st.so+"/"+st.action, st.body).Must(t, 200)
		if got := status(); got != st.want {
			t.Fatalf("after %s: status %s, want %s", st.action, got, st.want)
		}
	}
	events := member.Do("GET", "/api/v1/orders/"+id, nil).Must(t, 200).Get("events").([]any)
	if events[0].(map[string]any)["to_status"] != "delivered" {
		t.Fatalf("timeline wrong: %v", events[0])
	}
	if a := member.Do("GET", "/api/v1/activity?entity_type=supplier_order&entity_id="+mug.Str("id"), nil).Must(t, 200).Items(); len(a) < 4 {
		t.Fatalf("supplier order activity missing: %v", a)
	}
}

func TestOrderScopesAndSupplierPlacementPermission(t *testing.T) {
	e := newEnv(t)
	e.orderFixture()
	ordersOnly, _, _ := e.integrationKey("Orders only", "orders.write")
	both, _, _ := e.integrationKey("Orders and catalog", "orders.write", "catalog.read")
	ordersOnly.Do("GET", "/api/v1/catalog/products", nil).Must(t, 403)
	both.Do("GET", "/api/v1/catalog/products", nil).Must(t, 200)
	ordersOnly.Do("GET", "/api/v1/supplier-orders", nil).Must(t, 403)
	ordersOnly.Do("GET", "/api/v1/supplier-orders/queues", nil).Must(t, 403)
	ordersOnly.Do("GET", "/api/v1/products", nil).Must(t, 403)
	catalogOnly, _, _ := e.integrationKey("Catalog", "catalog.read")
	catalogOnly.Do("GET", "/api/v1/orders", nil).Must(t, 403)
	catalogOnly.Do("POST", "/api/v1/orders", orderBody("Z-1", line("MUG-1", 1, "1"))).Must(t, 403)
	ext, _ := e.pairExtension()
	ext.Do("GET", "/api/v1/orders", nil).Must(t, 403)
	ext.Do("POST", "/api/v1/orders", orderBody("Z-2", line("MUG-1", 1, "1"))).Must(t, 403)

	// The integrations config lists the new scopes and the order events; webhooks can subscribe to them.
	cfg := e.owner.Do("GET", "/api/v1/integrations/config", nil).Must(t, 200)
	if !strings.Contains(string(cfg.Body), `"supplier_orders.report"`) || !strings.Contains(string(cfg.Body), `"supplier_order.confirmed"`) {
		t.Fatalf("config lacks order scopes or events: %s", cfg.Body)
	}
	e.owner.Do("POST", "/api/v1/integrations/webhooks", map[string]any{"name": "Orders", "url": "https://hooks.example.com/orders",
		"events": []string{"order.received", "supplier_order.confirmed"}, "enabled": false}).Must(t, 201)
	e.owner.Do("POST", "/api/v1/integrations/webhooks", map[string]any{"name": "Bad", "url": "https://hooks.example.com/orders",
		"events": []string{"order.created"}, "enabled": false}).Must(t, 422)

	// Only people who may confirm can switch a supplier to automated placement.
	member := e.userWithRole("member", "member@example.com")
	member.Do("POST", "/api/v1/suppliers", map[string]any{"name": "Auto Co", "order_placement": "automation"}).Must(t, 403)
	sid := member.Do("POST", "/api/v1/suppliers", map[string]any{"name": "Manual Co"}).Must(t, 201).Str("id")
	sup := member.Do("GET", "/api/v1/suppliers/"+sid, nil).Must(t, 200)
	if sup.Str("order_placement") != "manual" {
		t.Fatalf("default placement: %s", sup.Body)
	}
	member.Do("PUT", "/api/v1/suppliers/"+sid, map[string]any{"name": "Manual Co", "order_placement": "automation"}).Must(t, 403)
	member.Do("PUT", "/api/v1/suppliers/"+sid, map[string]any{"name": "Manual Co", "order_placement": "manual", "city": "Ningbo"}).Must(t, 200)
	member.Do("PUT", "/api/v1/suppliers/"+sid, map[string]any{"name": "Manual Co", "order_placement": "robots"}).Must(t, 422)

	// Drafts follow their supplier's placement method until confirmed; the hash changes with it.
	pid := e.publish("Spoon", "SPOON-1", map[string]any{"supplier_id": sid, "source_url": "https://spoons.example.com/1", "cost": "1", "cost_currency": "EUR"})
	_ = pid
	o := member.Do("POST", "/api/v1/orders", orderBody("P-1", line("SPOON-1", 4, "3"))).Must(t, 201)
	before := supplierOrders(t, member, o.Str("id"))["Manual Co"]
	e.owner.Do("PUT", "/api/v1/suppliers/"+sid, map[string]any{"name": "Manual Co", "order_placement": "automation"}).Must(t, 200)
	after := supplierOrders(t, member, o.Str("id"))["Manual Co"]
	if after.Str("placement_method") != "automation" || after.Str("content_hash") == before.Str("content_hash") {
		t.Fatalf("draft did not follow the supplier: %s", after.Body)
	}
	e.owner.Do("POST", "/api/v1/supplier-orders/"+before.Str("id")+"/confirm", confirmBody(before, true)).Must(t, 409)
}

// lockedBuffer is an io.Writer safe for the server's concurrent log writes.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestEraseCustomerAndNoPersonalDataInLogs(t *testing.T) {
	logs := &lockedBuffer{}
	e := newEnvLogged(t, localWebhooks, logs)
	e.orderFixture()
	rc := newReceiver(t)
	e.owner.Do("POST", "/api/v1/integrations/webhooks", map[string]any{"name": "Shop", "url": rc.srv.URL,
		"events": []string{"order.received", "order.status_changed", "supplier_order.confirmed", "supplier_order.cancelled"}, "enabled": true}).Must(t, 201)
	shop, _, _ := e.integrationKey("Storefront", "orders.write")
	body := orderBody("E-1", line("TEE-RED-M", 1, "19.90"), line("UNKNOWN-SKU", 1, "4"))
	body["customer_note"] = "Please call Ada before delivery"
	o := shop.Do("POST", "/api/v1/orders", body).Must(t, 201)
	id := o.Str("id")
	// Error paths too: a conflict, validation failures, a refused confirmation.
	shop.Do("POST", "/api/v1/orders", orderBody("E-1", line("TEE-RED-M", 2, "19.90"))).Must(t, 409)
	bad := orderBody("E-2", line("TEE-RED-M", 1, "19.90"))
	bad["customer"] = map[string]any{"name": "Ada Lovelace", "email": "ada.lovelace@example.org@bad"}
	shop.Do("POST", "/api/v1/orders", bad).Must(t, 422)
	tee := supplierOrders(t, e.owner, id)["Tee Factory"]
	e.owner.Do("POST", "/api/v1/supplier-orders/"+tee.Str("id")+"/confirm", confirmBody(tee, false)).Must(t, 422)
	e.owner.Do("POST", "/api/v1/supplier-orders/"+tee.Str("id")+"/confirm", confirmBody(tee, true)).Must(t, 200)
	e.waitFor("deliveries", func() bool {
		return findEvent(rc, "order.received") != nil && findEvent(rc, "supplier_order.confirmed") != nil
	})
	if received := findEvent(rc, "order.received"); !strings.Contains(string(received.body), "ada.lovelace@example.org") {
		t.Fatalf("order.received should carry the customer for the storefront: %s", received.body)
	}

	// Erasure waits until no supplier order is open.
	if r := e.owner.Do("POST", "/api/v1/orders/"+id+"/erase-customer", nil).Must(t, 409); r.ErrorCode() != "supplier_order_open" {
		t.Fatalf("erased with an open supplier order: %s", r.Body)
	}
	pat := e.owner.Do("POST", "/api/v1/auth/tokens", map[string]any{"name": "script", "expires_in_days": 7, "scopes": []string{"orders.read", "orders.write"}}).Must(t, 201)
	patClient := e.client()
	patClient.Bearer = pat.Str("token")
	patClient.Do("POST", "/api/v1/orders/"+id+"/erase-customer", nil).Must(t, 403)
	shop.Do("POST", "/api/v1/orders/"+id+"/cancel", map[string]any{"reason": "Ada asked to cancel"}).Must(t, 200)
	erased := e.owner.Do("POST", "/api/v1/orders/"+id+"/erase-customer", nil).Must(t, 200)
	if erased.Str("customer.name") != "[erased]" || erased.Str("customer.email") != "" || erased.Str("shipping_address.line1") != "[erased]" ||
		erased.Str("shipping_address.country") != "GB" || erased.Get("customer_erased_at") == nil || erased.Str("customer_note") != "[erased]" {
		t.Fatalf("erasure wrong: %s", erased.Body)
	}
	e.owner.Do("POST", "/api/v1/orders/"+id+"/erase-customer", nil).Must(t, 409)
	// A repeated submission is still recognized and returns the erased order, not a new one.
	if again := shop.Do("POST", "/api/v1/orders", body).Must(t, 200); again.Str("id") != id || again.Str("customer.name") != "[erased]" {
		t.Fatalf("repeat after erasure: %s", again.Body)
	}
	so := e.owner.Do("GET", "/api/v1/supplier-orders/"+tee.Str("id"), nil).Must(t, 200)
	if so.Str("ship_to.name") != "[erased]" || so.Str("ship_to.phone") != "[erased]" {
		t.Fatalf("supplier order snapshot not erased: %s", so.Body)
	}
	if !strings.Contains(string(e.owner.Do("GET", "/api/v1/audit-logs?action=order.erase_customer", nil).Must(t, 200).Body), "order.erase_customer") {
		t.Fatal("erasure not audited")
	}
	e.worker.DrainWebhooks(t.Context())

	personal := []string{"ada.lovelace@example.org", "Ada Lovelace", "Analytical Engine", "N1 7AA", "7946", "Please call Ada", "Ada asked"}
	var dump strings.Builder
	for _, q := range []string{
		"SELECT coalesce(string_agg(row_to_json(a)::text, E'\\n'), '') FROM audit_logs a",
		"SELECT coalesce(string_agg(row_to_json(x)::text, E'\\n'), '') FROM order_events x",
		"SELECT coalesce(string_agg(payload::text, E'\\n'), '') FROM webhook_deliveries",
		"SELECT coalesce(string_agg(row_to_json(o)::text, E'\\n'), '') FROM orders o",
		"SELECT coalesce(string_agg(row_to_json(s)::text, E'\\n'), '') FROM supplier_orders s",
	} {
		var s string
		if err := e.db.Pool.QueryRow(t.Context(), q).Scan(&s); err != nil {
			t.Fatal(err)
		}
		dump.WriteString(s)
	}
	if e.countRows("SELECT count(*) FROM webhook_deliveries WHERE order_id = $1", id) < 3 {
		t.Fatal("expected stored order deliveries")
	}
	for _, p := range personal {
		if strings.Contains(dump.String(), p) {
			t.Errorf("personal data %q remains in the database after erasure or in the audit log", p)
		}
		if strings.Contains(logs.String(), p) {
			t.Errorf("personal data %q written to the server log", p)
		}
	}
	if !strings.Contains(logs.String(), "/api/v1/orders") {
		t.Fatal("the captured log does not contain request lines; the capture is not working")
	}
}
