package httpapi_test

import (
	"crypto/sha256"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/config"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/testutil"
)

// integrationKey creates an integration key and returns a client using it, the key ID and token.
func (e *env) integrationKey(name string, scopes ...string) (*testutil.Client, string, string) {
	e.t.Helper()
	res := e.owner.Do("POST", "/api/v1/integrations/keys", map[string]any{"name": name, "scopes": scopes}).Must(e.t, 201)
	c := e.client()
	c.Bearer = res.Str("token")
	return c, res.Str("key.id"), res.Str("token")
}

// publish creates a product and approves it in review, which puts it in the catalog.
func (e *env) publish(title, sku string, extra map[string]any) string {
	e.t.Helper()
	body := map[string]any{"title": title, "sku": sku, "price": "10.00", "price_currency": "EUR"}
	for k, v := range extra {
		body[k] = v
	}
	id := e.owner.Do("POST", "/api/v1/products", body).Must(e.t, 201).Str("id")
	e.owner.Do("POST", "/api/v1/products/"+id+"/review", map[string]any{"review_status": "approved"}).Must(e.t, 200)
	return id
}

// updateProduct replaces a product through the dashboard endpoint, keeping fields not in changes.
func (e *env) updateProduct(id string, changes map[string]any) {
	e.t.Helper()
	p := e.owner.Do("GET", "/api/v1/products/"+id, nil).Must(e.t, 200)
	in := map[string]any{}
	for _, k := range []string{"title", "source_url", "supplier_id", "brand", "sku", "model", "mpn", "gtin", "identifiers", "description",
		"specifications", "cost", "cost_currency", "price", "price_currency", "availability", "category", "status", "internal_notes", "custom_fields"} {
		in[k] = p.Get(k)
	}
	for k, v := range changes {
		in[k] = v
	}
	e.owner.Do("PUT", "/api/v1/products/"+id, in).Must(e.t, 200)
}

// changes reads the whole change feed after cursor and returns the events and the next cursor.
func changes(t *testing.T, c *testutil.Client, cursor string) ([]map[string]any, string) {
	t.Helper()
	var out []map[string]any
	for {
		res := c.Do("GET", "/api/v1/catalog/changes?limit=2&since="+cursor, nil).Must(t, 200)
		out = append(out, res.Items()...)
		next := res.Str("next_cursor")
		a, _ := strconv.ParseInt(cursor, 10, 64)
		b, err := strconv.ParseInt(next, 10, 64)
		if err != nil || b < a {
			t.Fatalf("cursor went backwards: %s -> %s", cursor, next)
		}
		cursor = next
		if res.Get("has_more") != true {
			return out, cursor
		}
	}
}

func typesFor(events []map[string]any, productID string) []string {
	var out []string
	for _, ev := range events {
		if ev["product_id"] == productID {
			out = append(out, ev["type"].(string))
		}
	}
	return out
}

func TestIntegrationKeysReachOnlyTheirEndpoints(t *testing.T) {
	e := newEnv(t)
	cat, catID, catToken := e.integrationKey("Storefront", "catalog.read")
	imp, impID, _ := e.integrationKey("Pipeline", "imports.write")

	cat.Do("GET", "/api/v1/catalog/products", nil).Must(t, 200)
	cat.Do("GET", "/api/v1/catalog/changes", nil).Must(t, 200)
	for _, path := range []string{"/api/v1/companies", "/api/v1/contacts", "/api/v1/suppliers", "/api/v1/leads", "/api/v1/opportunities",
		"/api/v1/tasks", "/api/v1/notes?entity_type=product&entity_id=" + catID, "/api/v1/products", "/api/v1/imports", "/api/v1/captures",
		"/api/v1/users", "/api/v1/roles", "/api/v1/settings", "/api/v1/auth/me", "/api/v1/auth/tokens", "/api/v1/audit-logs",
		"/api/v1/integrations/keys", "/api/v1/integrations/webhooks", "/api/v1/integrations/config", "/api/v1/system/info",
		"/api/v1/permissions", "/api/v1/users/directory", "/api/v1/search?q=a", "/api/v1/dashboard", "/api/v1/tags",
		"/api/v1/analysis/projects", "/api/v1/ai/providers", "/api/v1/ai/jobs", "/api/v1/admin/backups", "/api/v1/admin/export",
		"/api/v1/extension/config", "/api/v1/workspaces", "/api/v1/activity?entity_type=product&entity_id=" + catID} {
		if res := cat.Do("GET", path, nil); res.Status != 403 {
			t.Errorf("GET %s with an integration key: %d", path, res.Status)
		}
	}
	for _, w := range []struct{ method, path string }{
		{"POST", "/api/v1/companies"}, {"POST", "/api/v1/integrations/keys"}, {"PUT", "/api/v1/settings/ai"}, {"POST", "/api/v1/auth/logout"},
		{"POST", "/api/v1/imports/products"}, {"POST", "/api/v1/products"}, {"POST", "/api/v1/users"}, {"POST", "/api/v1/admin/backups"},
	} {
		if res := cat.Do(w.method, w.path, map[string]any{"name": "x"}); res.Status != 403 {
			t.Errorf("%s %s with a catalog key: %d", w.method, w.path, res.Status)
		}
	}
	// imports.write alone cannot read the catalog, but can submit products for review.
	imp.Do("GET", "/api/v1/catalog/products", nil).Must(t, 403)
	imp.Do("POST", "/api/v1/imports/products/check", map[string]any{"payload": productPayload("ERP-1")}).Must(t, 200)
	sub := imp.Do("POST", "/api/v1/imports/products", map[string]any{"payload": productPayload("ERP-1"), "tag_names": []string{"erp"}}).Must(t, 201)
	if sub.Str("product.status") != "draft" || sub.Str("product.review_status") != "pending" || sub.Str("import.source") != "api" ||
		sub.Get("import.created_by") != nil || sub.Get("product.created_by") != nil {
		t.Fatalf("integration import wrong: %s", sub.Body)
	}
	if items, _ := changes(t, cat, "0"); len(items) != 0 {
		t.Fatalf("an unapproved import must not reach the catalog: %v", items)
	}
	dup := imp.Do("POST", "/api/v1/imports/products", map[string]any{"payload": productPayload("ERP-1")}).Must(t, 202)
	if dup.Str("duplicates.0.id") != sub.Str("product.id") || dup.Get("duplicates.0.record") != nil {
		t.Fatalf("duplicate must be parked without leaking the stored record: %s", dup.Body)
	}
	if chk := imp.Do("POST", "/api/v1/imports/products/check", map[string]any{"payload": productPayload("ERP-1")}).Must(t, 200); chk.Get("duplicates.0.record") != nil {
		t.Fatalf("check leaks records: %s", chk.Body)
	}
	imp.Do("POST", "/api/v1/imports/products", map[string]any{"payload": productPayload("ERP-1"), "decision": "update_existing",
		"target_product_id": sub.Str("product.id")}).Must(t, 403)
	imp.Do("POST", "/api/v1/imports/products", map[string]any{"payload": productPayload("ERP-2"), "source": "extension"}).Must(t, 422)
	withSupplier := productPayload("ERP-3")
	withSupplier["url"] = "https://other.example.org/p/3"
	withSupplier["supplier_name"] = "Brand New Supplier Ltd"
	imp.Do("POST", "/api/v1/imports/products", map[string]any{"payload": withSupplier, "create_supplier": true}).Must(t, 403)

	// The audit log names the key, not a person.
	logs := e.owner.Do("GET", "/api/v1/audit-logs?action=import.submit", nil).Must(t, 200).Items()
	if len(logs) == 0 || !strings.Contains(logs[0]["actor_label"].(string), "integration key Pipeline") ||
		logs[0]["actor_integration_key_id"] != impID || logs[0]["actor_user_id"] != nil {
		t.Fatalf("import not attributed to the key: %v", logs)
	}
	// Use is counted per request and audited once per hour and address.
	for i := 0; i < 3; i++ {
		cat.Do("GET", "/api/v1/catalog/products", nil).Must(t, 200)
	}
	uses := 0
	for _, l := range e.owner.Do("GET", "/api/v1/audit-logs?action=integration_key.use", nil).Must(t, 200).Items() {
		if l["entity_id"] == catID {
			uses++
		}
	}
	if uses != 1 {
		t.Fatalf("expected one use audit event, got %d", uses)
	}
	list := e.owner.Do("GET", "/api/v1/integrations/keys", nil).Must(t, 200)
	if strings.Contains(string(list.Body), catToken) || strings.Contains(string(list.Body), "token_hash") {
		t.Fatal("key list leaks the token")
	}
	for _, k := range list.Items() {
		if k["id"] == catID && (k["requests_today"].(float64) < 40 || k["last_used_at"] == nil || !strings.HasPrefix(k["prefix"].(string), "reai_")) {
			t.Fatalf("usage not recorded: %v", k)
		}
	}
	var hash []byte
	if err := e.db.Pool.QueryRow(t.Context(), "SELECT token_hash FROM integration_keys WHERE id = $1", catID).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if sum := sha256.Sum256([]byte(catToken)); string(hash) != string(sum[:]) {
		t.Fatal("token is not stored as its SHA-256 hash")
	}
}

func TestIntegrationKeyLifecycle(t *testing.T) {
	e := newEnv(t)
	for _, bad := range []map[string]any{
		{"name": "x", "scopes": []string{"crm.read"}}, {"name": "x", "scopes": []string{"catalog.read_costs"}},
		{"name": "x", "scopes": []string{}}, {"name": "", "scopes": []string{"catalog.read"}},
		{"name": "x", "scopes": []string{"catalog.read"}, "expires_in_days": 1000},
	} {
		e.owner.Do("POST", "/api/v1/integrations/keys", bad).Must(t, 422)
	}
	// Only administrators, and only from the web application (not with an API token).
	viewer := e.userWithRole("viewer", "viewer@example.com")
	viewer.Do("POST", "/api/v1/integrations/keys", map[string]any{"name": "x", "scopes": []string{"catalog.read"}}).Must(t, 403)
	pat := e.owner.Do("POST", "/api/v1/auth/tokens", map[string]any{"name": "script", "expires_in_days": 7, "scopes": []string{"settings.manage", "products.read"}}).Must(t, 201)
	patClient := e.client()
	patClient.Bearer = pat.Str("token")
	patClient.Do("POST", "/api/v1/integrations/keys", map[string]any{"name": "x", "scopes": []string{"catalog.read"}}).Must(t, 403)
	// Users who can read products may use the catalog API themselves (they already see costs).
	if p := patClient.Do("GET", "/api/v1/catalog/products", nil).Must(t, 200); p.Get("items") == nil {
		t.Fatal("PAT with products.read cannot read the catalog")
	}

	// Keys belong to the organization: downgrading or deleting the creator neither breaks nor widens them.
	admin2 := e.userWithRole("admin", "admin2@example.com")
	res := admin2.Do("POST", "/api/v1/integrations/keys", map[string]any{"name": "ERP", "scopes": []string{"catalog.read"}, "expires_in_days": 0}).Must(t, 201)
	key := e.client()
	key.Bearer = res.Str("token")
	if res.Get("key.expires_at") != nil || res.Str("key.status") != "active" {
		t.Fatalf("key without expiry wrong: %s", res.Body)
	}
	var adminID, viewerRole string
	for _, u := range e.owner.Do("GET", "/api/v1/users", nil).Must(t, 200).Items() {
		if u["email"] == "admin2@example.com" {
			adminID = u["id"].(string)
		}
	}
	for _, r := range e.owner.Do("GET", "/api/v1/roles", nil).Must(t, 200).Items() {
		if r["key"] == "viewer" {
			viewerRole = r["id"].(string)
		}
	}
	e.owner.Do("PUT", "/api/v1/users/"+adminID, map[string]any{"display_name": "Former admin", "role_id": viewerRole, "is_active": true}).Must(t, 200)
	key.Do("GET", "/api/v1/catalog/changes", nil).Must(t, 200)
	key.Do("POST", "/api/v1/imports/products/check", map[string]any{"payload": productPayload("X")}).Must(t, 403)
	e.owner.Do("DELETE", "/api/v1/users/"+adminID, nil).Must(t, 204)
	key.Do("GET", "/api/v1/catalog/changes", nil).Must(t, 200)
	for _, k := range e.owner.Do("GET", "/api/v1/integrations/keys", nil).Must(t, 200).Items() {
		if k["id"] == res.Str("key.id") && (k["creator_removed"] != true || k["created_by"] != "admin2@example.com") {
			t.Fatalf("creator not shown after deletion: %v", k)
		}
	}

	// Revocation takes effect immediately and is idempotent.
	e.owner.Do("DELETE", "/api/v1/integrations/keys/"+res.Str("key.id"), nil).Must(t, 204)
	if r := key.Do("GET", "/api/v1/catalog/changes", nil).Must(t, 401); r.ErrorCode() != "token_expired" {
		t.Fatalf("revoked key: %s", r.Body)
	}
	e.owner.Do("DELETE", "/api/v1/integrations/keys/"+res.Str("key.id"), nil).Must(t, 204)
	e.owner.Do("DELETE", "/api/v1/integrations/keys/00000000-0000-4000-8000-000000000000", nil).Must(t, 404)

	// Expired keys stop working.
	exp, expID, _ := e.integrationKey("Short lived", "catalog.read")
	exp.Do("GET", "/api/v1/catalog/products", nil).Must(t, 200)
	if _, err := e.db.Pool.Exec(t.Context(), "UPDATE integration_keys SET expires_at = now() - interval '1 minute' WHERE id = $1", expID); err != nil {
		t.Fatal(err)
	}
	exp.Do("GET", "/api/v1/catalog/products", nil).Must(t, 401)
	bad := e.client()
	bad.Bearer = "reai_not-a-real-key"
	bad.Do("GET", "/api/v1/catalog/products", nil).Must(t, 401)
}

func TestIntegrationKeysAreRateLimitedPerKey(t *testing.T) {
	e := newEnvWith(t, func(c *config.Config) { c.IntegrationRateLimit = 6 })
	a, _, _ := e.integrationKey("Busy", "catalog.read")
	b, _, _ := e.integrationKey("Quiet", "catalog.read")
	limited := 0
	for i := 0; i < 20; i++ {
		res := a.Do("GET", "/api/v1/catalog/changes", nil)
		if res.Status == 429 {
			if res.ErrorCode() != "rate_limited" || res.Header.Get("Retry-After") == "" {
				t.Fatalf("wrong error: %s", res.Body)
			}
			limited++
		}
	}
	if limited == 0 {
		t.Fatal("integration key was not rate limited")
	}
	b.Do("GET", "/api/v1/catalog/changes", nil).Must(t, 200)
}

func TestCatalogHidesCostsWithoutScope(t *testing.T) {
	e := newEnv(t)
	supplier := e.owner.Do("POST", "/api/v1/suppliers", map[string]any{"name": "Ningbo Bottles"}).Must(t, 201).Str("id")
	id := e.publish("Bottle", "B-1", map[string]any{"cost": "4.20", "cost_currency": "EUR", "supplier_id": supplier,
		"source_url": "https://supplier.example.com/p/1", "description": "Steel bottle"})
	public, _, _ := e.integrationKey("Shop", "catalog.read")
	costs, _, _ := e.integrationKey("ERP", "catalog.read", "catalog.read_costs")

	p := public.Do("GET", "/api/v1/catalog/products/"+id, nil).Must(t, 200)
	if p.Get("sourcing") != nil || strings.Contains(string(p.Body), `"4.2"`) || strings.Contains(string(p.Body), "supplier.example.com") ||
		strings.Contains(string(p.Body), "Ningbo") || p.Str("pricing.price") != "10" || p.Str("description") != "Steel bottle" {
		t.Fatalf("public catalog leaks sourcing: %s", p.Body)
	}
	c := costs.Do("GET", "/api/v1/catalog/products/"+id, nil).Must(t, 200)
	if c.Str("sourcing.cost") != "4.2" || c.Str("sourcing.supplier.name") != "Ningbo Bottles" || c.Str("sourcing.source_url") != "https://supplier.example.com/p/1" ||
		c.Str("sourcing.source_domain") != "supplier.example.com" {
		t.Fatalf("costs scope misses sourcing: %s", c.Body)
	}
	for _, list := range []*testutil.Response{public.Do("GET", "/api/v1/catalog/products", nil).Must(t, 200),
		public.Do("GET", "/api/v1/catalog/changes", nil).Must(t, 200)} {
		if strings.Contains(string(list.Body), "sourcing") || strings.Contains(string(list.Body), `"4.2"`) || strings.Contains(string(list.Body), "Ningbo") {
			t.Fatalf("list leaks sourcing: %s", list.Body)
		}
	}
	public.Do("GET", "/api/v1/catalog/products?source_domain=supplier.example.com", nil).Must(t, 403)
	if n := len(costs.Do("GET", "/api/v1/catalog/products?source_domain=supplier.example.com", nil).Must(t, 200).Items()); n != 1 {
		t.Fatalf("source domain filter found %d", n)
	}

	// A cost-only change is invisible without the scope, and its fields are never named.
	_, pubCursor := changes(t, public, "0")
	_, costCursor := changes(t, costs, "0")
	e.updateProduct(id, map[string]any{"cost": "3.90"})
	pubEvents, _ := changes(t, public, pubCursor)
	costEvents, _ := changes(t, costs, costCursor)
	if len(pubEvents) != 0 {
		t.Fatalf("sourcing-only change visible without catalog.read_costs: %v", pubEvents)
	}
	if len(costEvents) != 1 || costEvents[0]["type"] != "product.updated" || !slices.Contains(toStrings(costEvents[0]["changed_fields"]), "sourcing.cost") {
		t.Fatalf("cost change missing for the costs scope: %v", costEvents)
	}
	e.updateProduct(id, map[string]any{"cost": "3.50", "title": "Steel bottle"})
	pubEvents, _ = changes(t, public, pubCursor)
	if len(pubEvents) != 1 || !slices.Equal(toStrings(pubEvents[0]["changed_fields"]), []string{"title"}) {
		t.Fatalf("mixed change should show only public fields: %v", pubEvents)
	}
}

func TestCatalogKeysetPaginationIsStableUnderInserts(t *testing.T) {
	e := newEnv(t)
	var original []string
	for i := 0; i < 7; i++ {
		original = append(original, e.publish(fmt.Sprintf("Product %d", i), fmt.Sprintf("SKU-%d", i), nil))
	}
	// A draft never appears.
	e.owner.Do("POST", "/api/v1/products", map[string]any{"title": "Draft only"}).Must(t, 201)
	key, _, _ := e.integrationKey("Sync", "catalog.read")
	var seen, inserted []string
	cursor := ""
	for page := 0; page < 20; page++ {
		path := "/api/v1/catalog/products?limit=3&fields=summary"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		res := key.Do("GET", path, nil).Must(t, 200)
		for _, it := range res.Items() {
			seen = append(seen, it["id"].(string))
			if it["variants"] != nil || it["description"] != nil {
				t.Fatalf("summary includes full fields: %v", it)
			}
		}
		switch page {
		case 0:
			inserted = append(inserted, e.publish("Inserted during sync", "NEW-1", nil))
		case 1:
			// Concurrent writers outside the API (another process, the pricing engine, SQL).
			var wg sync.WaitGroup
			errs := make(chan error, 3)
			for i := 0; i < 3; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					var id string
					errs <- e.db.Pool.QueryRow(t.Context(), `INSERT INTO products (original_title, title, normalized_title, sku, status, review_status)
						VALUES ($1, $1, lower($1), $2, 'active', 'approved') RETURNING id`, fmt.Sprintf("Concurrent %d", i), fmt.Sprintf("C-%d", i)).Scan(&id)
				}(i)
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				if err != nil {
					t.Fatal(err)
				}
			}
		}
		if res.Get("has_more") != true {
			if res.Get("next_cursor") != nil {
				t.Fatal("last page must not have a next cursor")
			}
			break
		}
		cursor = res.Str("next_cursor")
	}
	counts := map[string]int{}
	for _, id := range seen {
		counts[id]++
	}
	for id, n := range counts {
		if n != 1 {
			t.Fatalf("product %s returned %d times", id, n)
		}
	}
	for _, id := range original {
		if counts[id] != 1 {
			t.Fatalf("original product %s missed by the pagination", id)
		}
	}
	if len(seen) != 7+1+3 || counts[inserted[0]] != 1 {
		t.Fatalf("expected originals plus products inserted during the sync, got %d", len(seen))
	}
	// Products inserted during the sync come after every product that existed before.
	last := 0
	for i, id := range seen {
		if slices.Contains(original, id) {
			last = i
		}
	}
	if last != 6 {
		t.Fatalf("an original product was ordered after an inserted one: %v", seen)
	}
	key.Do("GET", "/api/v1/catalog/products?cursor=abc", nil).Must(t, 422)
	key.Do("GET", "/api/v1/catalog/products?fields=everything", nil).Must(t, 422)
	key.Do("GET", "/api/v1/catalog/products?availability=sold", nil).Must(t, 422)
}

func TestCatalogFiltersLookupsAndETags(t *testing.T) {
	e := newEnv(t)
	a := e.publish("Red mug", "MUG-R", map[string]any{"category": "Kitchen", "availability": "in_stock"})
	b := e.publish("Blue mug", "MUG-B", map[string]any{"category": "Kitchen", "availability": "out_of_stock"})
	e.publish("Shared SKU one", "DUP", nil)
	e.publish("Shared SKU two", "DUP", nil)
	e.owner.Do("POST", "/api/v1/products/"+b+"/variants", map[string]any{"title": "Large", "sku": "MUG-B-L", "price": "12", "currency": "EUR"}).Must(t, 201)
	e.owner.Do("PUT", "/api/v1/record-tags", map[string]any{"entity_type": "product", "entity_id": a, "tag_names": []string{"Gift"}}).Must(t, 200)
	key, _, _ := e.integrationKey("Shop", "catalog.read")
	count := func(q string) int { return len(key.Do("GET", "/api/v1/catalog/products?"+q, nil).Must(t, 200).Items()) }
	if count("category=kitchen") != 2 || count("availability=in_stock") != 1 || count("tag=gift") != 1 || count("sku=mug-b-l") != 1 || count("sku=DUP") != 2 {
		t.Fatal("filters wrong")
	}
	if since := time.Now().Add(time.Hour).UTC().Format(time.RFC3339); count("updated_since="+url.QueryEscape(since)) != 0 {
		t.Fatal("updated_since in the future should match nothing")
	}
	bySKU := key.Do("GET", "/api/v1/catalog/products/by-sku/MUG-B-L", nil).Must(t, 200)
	if bySKU.Str("id") != b || bySKU.Str("variants.0.sku") != "MUG-B-L" || bySKU.Str("variants.0.pricing.price") != "12" {
		t.Fatalf("variant SKU lookup wrong: %s", bySKU.Body)
	}
	if key.Do("GET", "/api/v1/catalog/products/by-sku/mug-r", nil).Must(t, 200).Str("tags.0") != "Gift" {
		t.Fatal("tag missing")
	}
	if key.Do("GET", "/api/v1/catalog/products/by-sku/DUP", nil).Must(t, 409).ErrorCode() != "sku_ambiguous" {
		t.Fatal("ambiguous SKU not reported")
	}
	key.Do("GET", "/api/v1/catalog/products/by-sku/NOPE", nil).Must(t, 404)
	draft := e.owner.Do("POST", "/api/v1/products", map[string]any{"title": "Hidden"}).Must(t, 201).Str("id")
	key.Do("GET", "/api/v1/catalog/products/"+draft, nil).Must(t, 404)

	// ETags: unchanged data answers 304, a change gives a new representation.
	first := key.Do("GET", "/api/v1/catalog/products/"+a, nil).Must(t, 200)
	etag := first.Header.Get("ETag")
	if !strings.HasPrefix(etag, `W/"`) {
		t.Fatalf("no ETag: %v", first.Header)
	}
	key.Headers["If-None-Match"] = etag
	if r := key.Do("GET", "/api/v1/catalog/products/"+a, nil).Must(t, 304); len(r.Body) != 0 {
		t.Fatal("304 with a body")
	}
	listETag := key.Do("GET", "/api/v1/catalog/products?limit=10", nil).Header.Get("ETag")
	key.Headers["If-None-Match"] = listETag
	key.Do("GET", "/api/v1/catalog/products?limit=10", nil).Must(t, 304)
	e.owner.Do("POST", "/api/v1/products/"+a+"/images", map[string]any{"url": "https://cdn.example.com/red.jpg"}).Must(t, 201)
	key.Do("GET", "/api/v1/catalog/products?limit=10", nil).Must(t, 200)
	key.Headers["If-None-Match"] = etag
	changed := key.Do("GET", "/api/v1/catalog/products/"+a, nil).Must(t, 200)
	if changed.Header.Get("ETag") == etag || changed.Str("primary_image_url") != "https://cdn.example.com/red.jpg" {
		t.Fatalf("image change not reflected: %s", changed.Body)
	}
	delete(key.Headers, "If-None-Match")
}

func TestChangeFeedOrderingTombstonesAndMirror(t *testing.T) {
	e := newEnv(t)
	key, _, _ := e.integrationKey("Mirror", "catalog.read")
	a := e.publish("Lamp", "L-1", map[string]any{"availability": "in_stock"})
	b := e.publish("Chair", "C-1", nil)
	e.updateProduct(a, map[string]any{"price": "12.00"})
	e.updateProduct(a, map[string]any{"availability": "out_of_stock"})
	e.updateProduct(a, map[string]any{"price": "13.00", "availability": "in_stock"})
	e.updateProduct(a, map[string]any{"title": "Desk lamp"})
	e.updateProduct(a, map[string]any{"internal_notes": "not part of the catalog"})
	e.owner.Do("POST", "/api/v1/products/"+b+"/review", map[string]any{"review_status": "needs_changes"}).Must(t, 200)
	c := e.publish("Table", "T-1", nil)
	e.owner.Do("DELETE", "/api/v1/products/"+c, nil).Must(t, 204)
	// A draft product never shows up, whatever happens to it.
	d := e.owner.Do("POST", "/api/v1/products", map[string]any{"title": "Draft"}).Must(t, 201).Str("id")
	e.owner.Do("DELETE", "/api/v1/products/"+d, nil).Must(t, 204)

	events, cursor := changes(t, key, "0")
	if got := typesFor(events, a); !slices.Equal(got, []string{"product.published", "product.price_changed", "product.availability_changed",
		"product.price_changed", "product.availability_changed", "product.updated"}) {
		t.Fatalf("lamp events: %v", got)
	}
	if got := typesFor(events, b); !slices.Equal(got, []string{"product.published", "product.unpublished"}) {
		t.Fatalf("chair events: %v", got)
	}
	if got := typesFor(events, c); !slices.Equal(got, []string{"product.published", "product.deleted"}) {
		t.Fatalf("table events: %v", got)
	}
	if len(typesFor(events, d)) != 0 {
		t.Fatal("draft product leaked into the feed")
	}
	prev := int64(0)
	ids := map[string]bool{}
	for _, ev := range events {
		var n int64
		fmt.Sscan(ev["cursor"].(string), &n)
		if n <= prev {
			t.Fatalf("feed not ordered: %v", events)
		}
		prev = n
		if ids[ev["id"].(string)] {
			t.Fatal("event id repeated")
		}
		ids[ev["id"].(string)] = true
		if (ev["type"] == "product.unpublished" || ev["type"] == "product.deleted") && ev["product"] != nil {
			t.Fatalf("tombstone with a product: %v", ev)
		}
	}
	for _, ev := range events {
		if ev["type"] == "product.deleted" && ev["sku"] != "T-1" {
			t.Fatalf("tombstone lost the SKU: %v", ev)
		}
	}

	// Applying the feed in order mirrors the catalog exactly.
	mirror := map[string]map[string]any{}
	apply := func(evs []map[string]any) {
		for _, ev := range evs {
			pid := ev["product_id"].(string)
			if ev["type"] == "product.unpublished" || ev["type"] == "product.deleted" {
				delete(mirror, pid)
			} else if p, ok := ev["product"].(map[string]any); ok {
				mirror[pid] = p
			}
		}
	}
	apply(events)
	e.owner.Do("POST", "/api/v1/products/"+b+"/review", map[string]any{"review_status": "approved"}).Must(t, 200)
	more, _ := changes(t, key, cursor)
	apply(more)
	list := key.Do("GET", "/api/v1/catalog/products", nil).Must(t, 200).Items()
	if len(list) != len(mirror) {
		t.Fatalf("mirror has %d products, catalog %d", len(mirror), len(list))
	}
	for _, p := range list {
		m := mirror[p["id"].(string)]
		if m == nil || m["title"] != p["title"] || m["version"] != p["version"] {
			t.Fatalf("mirror out of date for %v: %v", p["id"], m)
		}
	}
	if mirror[a]["title"] != "Desk lamp" || mirror[a]["pricing"].(map[string]any)["price"] != "13" {
		t.Fatalf("lamp mirrored wrong: %v", mirror[a])
	}

	// Cursors outside the retained feed ask for a resync.
	key.Do("GET", "/api/v1/catalog/changes?since=999999", nil).Must(t, 410)
	key.Do("GET", "/api/v1/catalog/changes?since=x", nil).Must(t, 422)
	if n := len(key.Do("GET", "/api/v1/catalog/changes?fields=none", nil).Must(t, 200).Items()); n == 0 {
		t.Fatal("fields=none returned nothing")
	}
	head := key.Do("GET", "/api/v1/catalog/products", nil).Must(t, 200).Str("change_cursor")
	e.cfg.CatalogChangeRetention = -time.Minute
	e.worker.PruneIntegrations(t.Context())
	if r := key.Do("GET", "/api/v1/catalog/changes?since=0", nil).Must(t, 410); r.ErrorCode() != "cursor_expired" {
		t.Fatalf("expected cursor_expired: %s", r.Body)
	}
	key.Do("GET", "/api/v1/catalog/changes?since="+head, nil).Must(t, 200)
}

// TestChangeCaptureCoversEveryWritePath checks that the triggers see writes from every path:
// dashboard edits, variants, images, tags, suppliers, imports, bulk actions, merges, AI apply and
// direct SQL (as the pricing engine or another process would write).
func TestChangeCaptureCoversEveryWritePath(t *testing.T) {
	e := newEnv(t)
	key, _, _ := e.integrationKey("Watcher", "catalog.read", "catalog.read_costs")
	_, cursor := changes(t, key, "0")
	expect := func(what, productID string, want ...string) {
		t.Helper()
		var events []map[string]any
		events, cursor = changes(t, key, cursor)
		if got := typesFor(events, productID); !slices.Equal(got, want) {
			t.Fatalf("%s: got %v, want %v (all: %v)", what, got, want, events)
		}
	}
	supplier := e.owner.Do("POST", "/api/v1/suppliers", map[string]any{"name": "Acme Parts"}).Must(t, 201).Str("id")
	p := e.publish("Kettle", "K-1", map[string]any{"supplier_id": supplier})
	expect("review approval", p, "product.published")

	e.updateProduct(p, map[string]any{"description": "Electric kettle"})
	expect("dashboard edit", p, "product.updated")
	v := e.owner.Do("POST", "/api/v1/products/"+p+"/variants", map[string]any{"title": "1.7 L", "sku": "K-1-17", "price": "30", "currency": "EUR"}).Must(t, 201).Str("id")
	expect("variant create", p, "product.updated")
	e.owner.Do("PUT", "/api/v1/products/"+p+"/variants/"+v, map[string]any{"title": "1.7 L", "sku": "K-1-17", "price": "28", "currency": "EUR"}).Must(t, 200)
	expect("variant price", p, "product.price_changed")
	e.owner.Do("PUT", "/api/v1/products/"+p+"/variants/"+v, map[string]any{"title": "1.7 L", "sku": "K-1-17", "price": "28", "currency": "EUR", "availability": "preorder"}).Must(t, 200)
	expect("variant availability", p, "product.availability_changed")
	img := e.owner.Do("POST", "/api/v1/products/"+p+"/images", map[string]any{"url": "https://cdn.example.com/k1.jpg"}).Must(t, 201).Str("id")
	img2 := e.owner.Do("POST", "/api/v1/products/"+p+"/images", map[string]any{"url": "https://cdn.example.com/k2.jpg"}).Must(t, 201).Str("id")
	expect("image add", p, "product.updated", "product.updated")
	e.owner.Do("POST", "/api/v1/products/"+p+"/images/"+img2+"/primary", nil).Must(t, 200)
	expect("primary image", p, "product.updated")
	if first := key.Do("GET", "/api/v1/catalog/products/"+p, nil).Must(t, 200); first.Str("images.0.id") != img2 || first.Get("images.0.is_primary") != true {
		t.Fatalf("primary image must come first: %s", first.Body)
	}
	e.owner.Do("DELETE", "/api/v1/products/"+p+"/images/"+img, nil).Must(t, 204)
	expect("image delete", p, "product.updated")
	e.owner.Do("DELETE", "/api/v1/products/"+p+"/variants/"+v, nil).Must(t, 204)
	expect("variant delete", p, "product.updated")
	tags := e.owner.Do("PUT", "/api/v1/record-tags", map[string]any{"entity_type": "product", "entity_id": p, "tag_names": []string{"Sale"}}).Must(t, 200)
	expect("tag link", p, "product.updated")
	e.owner.Do("PUT", "/api/v1/record-tags", map[string]any{"entity_type": "product", "entity_id": p, "tag_names": []string{"Sale"}}).Must(t, 200)
	expect("same tags again", p)
	tagID := tags.Items()[0]["id"].(string)
	e.owner.Do("PUT", "/api/v1/tags/"+tagID, map[string]any{"name": "Summer sale", "color": "#ff0000"}).Must(t, 200)
	expect("tag rename", p, "product.updated")
	e.owner.Do("PUT", "/api/v1/suppliers/"+supplier, map[string]any{"name": "Acme Parts GmbH"}).Must(t, 200)
	expect("supplier rename", p, "product.updated")

	// Import refresh of an existing product (a new capture of the same page).
	imported := e.owner.Do("POST", "/api/v1/imports/products", map[string]any{"payload": productPayload("IMP-1")}).Must(t, 201).Str("product.id")
	e.owner.Do("POST", "/api/v1/products/"+imported+"/review", map[string]any{"review_status": "approved"}).Must(t, 200)
	expect("import approved", imported, "product.published")
	refreshed := productPayload("IMP-1")
	refreshed["price"] = "$21.50"
	e.owner.Do("POST", "/api/v1/imports/products", map[string]any{"payload": refreshed, "decision": "update_existing", "target_product_id": imported}).Must(t, 201)
	expect("import refresh", imported, "product.price_changed")

	// Bulk actions.
	bulk := func(action, value string, ids ...string) {
		e.owner.Do("POST", "/api/v1/products/bulk", map[string]any{"action": action, "ids": ids, "value": value}).Must(t, 200)
	}
	bulk("set_status", "draft", p)
	expect("bulk unpublish", p, "product.unpublished")
	bulk("set_review", "approved", p)
	expect("bulk approve", p, "product.published")
	bulk("add_tag", tagID, imported)
	expect("bulk tag", imported, "product.updated")

	// Merge: the merged-away product is deleted, the target changes.
	other := e.publish("Kettle duplicate", "K-2", map[string]any{"description": "dup"})
	_, cursor = changes(t, key, cursor)
	e.owner.Do("POST", "/api/v1/products/"+p+"/merge", map[string]any{"source_ids": []string{other}}).Must(t, 200)
	events, next := changes(t, key, cursor)
	if !slices.Equal(typesFor(events, other), []string{"product.deleted"}) {
		t.Fatalf("merge source: %v", events)
	}
	cursor = next

	// AI apply.
	srv := fakeOllama(t, nil)
	e.owner.Do("POST", "/api/v1/ai/providers", map[string]any{"name": "Local", "kind": "ollama", "base_url": srv.URL, "model": "m", "enabled": true}).Must(t, 201)
	job := e.owner.Do("POST", "/api/v1/ai/jobs", map[string]any{"tool": "normalize_product", "entity_type": "product", "entity_id": imported}).Must(t, 202)
	e.worker.Drain(t.Context())
	e.owner.Do("POST", "/api/v1/ai/jobs/"+job.Str("id")+"/apply", map[string]any{"fields": []string{"title"}}).Must(t, 200)
	expect("AI apply", imported, "product.updated")

	// Direct SQL, as a repricing job or another service would write.
	if _, err := e.db.Pool.Exec(t.Context(), "UPDATE products SET price = price + 1 WHERE id = $1", imported); err != nil {
		t.Fatal(err)
	}
	expect("SQL repricing", imported, "product.price_changed")
	if _, err := e.db.Pool.Exec(t.Context(), "UPDATE products SET last_checked_at = now() + interval '1 day' WHERE id = $1", imported); err != nil {
		t.Fatal(err)
	}
	expect("non-catalog column", imported)

	bulk("delete", "", imported)
	expect("bulk delete", imported, "product.deleted")
}
