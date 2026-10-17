# API

The REST API lives under `/api/v1`. Its OpenAPI 3 document is generated from the Go request and
response types and served at `/api/v1/openapi.json`; the committed copy is
`packages/api-types/openapi.json`, and tests fail when the two differ. Every integration test
response is validated against the document.

## Authentication

| Method | Use | How |
| --- | --- | --- |
| Session cookie | Dashboard | `POST /api/v1/auth/login` sets `rea_session` (`__Host-rea_session` on HTTPS) and returns `csrf_token`. Send it as `X-CSRF-Token` on every non-GET request. |
| Personal API token | Scripts and integrations | Create under Profile → API tokens. Send `Authorization: Bearer reap_…`. Same permissions as the user. |
| Extension tokens | Browser extension | Pairing and refresh endpoints under `/api/v1/extension`. Access tokens (`reaa_…`) last one hour. |
| Integration keys | Your storefront, ERP and other systems | Created by administrators under Settings → Integrations. Send `Authorization: Bearer reai_…`. Organization-owned, scoped (`catalog.read`, `catalog.read_costs`, `imports.write`, `orders.write`, `supplier_orders.report`) and limited to the catalog, product import and order intake and reporting endpoints. See [catalog-api.md](catalog-api.md) and [orders.md](orders.md). |

Request bodies must be `application/json` (uploads use `multipart/form-data`) and may not contain
unknown fields.

## Conventions

- Lists return `{"items": [...], "total", "limit", "offset"}`; `limit` is at most 200. The catalog
  endpoints use keyset pagination instead (`next_cursor`, `has_more`) and return ETags.
- Errors return `{"error": {"code": "validation_failed", "message": "...", "fields": {"name": "is required"}}}`
  with an appropriate status (400, 401, 403, 404, 409, 413, 415, 422, 429, 503).
- Timestamps are RFC 3339 UTC. Money values are decimal strings with a separate ISO 4217 currency.
- Exports accept `format=csv|json` and the same filters as the list endpoint.

## Catalog API and webhooks

External systems read published products (status `active`, review `approved`) from
`/api/v1/catalog/products`, follow an ordered change feed with tombstones at
`/api/v1/catalog/changes`, and can receive the same events as signed webhooks. Cost, supplier and
source fields need the `catalog.read_costs` scope. Keys and webhooks are managed under
`/api/v1/integrations` (administrators, browser session). The full guide, including the sync
algorithm, payloads, signature verification and retry rules, is in
[catalog-api.md](catalog-api.md).

```bash
curl -fsS -H "Authorization: Bearer $REA_KEY" "$BASE/api/v1/catalog/products?limit=100"
curl -fsS -H "Authorization: Bearer $REA_KEY" "$BASE/api/v1/catalog/changes?since=0"
```

## Orders

Customer orders arrive at `POST /api/v1/orders` (integration keys with `orders.write`, or people
with `orders.write`) and are idempotent per key and `external_id` (`200` with the existing order on
a repeat, `409 order_conflict` when the contents differ). REA prepares draft supplier orders
(`/api/v1/supplier-orders`); a draft is handed off only when a person with `orders.place` confirms
it from a dashboard session with the `content_hash` they reviewed — integration keys, personal and
extension tokens cannot confirm. Placements, failures and shipments are recorded by people or
reported by a `supplier_orders.report` key. Order webhooks (`order.*`, `supplier_order.*`) carry
customer personal data. Full guide: [orders.md](orders.md).

```bash
curl -fsS -H "Authorization: Bearer $REA_KEY" -H 'Content-Type: application/json' "$BASE/api/v1/orders" \
  -d '{"external_id":"1001","currency":"EUR","shipping_address":{"name":"Ada Lovelace","line1":"12 Example Street","city":"London","postal_code":"N1 7AA","country":"GB"},"lines":[{"sku":"MUG-RED-L","quantity":2,"unit_price":"19.90"}]}'
curl -fsS -H "Authorization: Bearer $REA_KEY" "$BASE/api/v1/orders/by-external-id/1001"
```

## Example

```bash
BASE=http://localhost
TOKEN=reap_your_token
curl -fsS -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/products?review_status=pending&limit=20"
curl -fsS -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"payload":{"url":"https://shop.example/p/1","title":"Steel bottle 750 ml","price":"19.90","currency":"EUR"},"source":"api","decision":"","create_supplier":false,"tag_names":[]}' \
  "$BASE/api/v1/imports/products/check"
```

## Product prices

Products imported from marketplaces carry three prices: `source_price` (the supplier's price
observed at import), `cost` (derived from it in the base currency unless `cost_manual`) and the
selling `price` with `compare_at_price`, set by the first matching pricing rule
(`pricing_rule_id`, `pricing_rule_name`, `priced_at`) unless `price_locked`. Systems that consume
the catalog should read `price`/`price_currency` as the selling price. `GET /api/v1/products/{id}`
adds `pricing`, the step-by-step breakdown under the current rules.

| Endpoint | Purpose |
| --- | --- |
| `GET/POST /api/v1/pricing/rules`, `GET/PUT/DELETE /api/v1/pricing/rules/{id}`, `POST /api/v1/pricing/rules/reorder` | Pricing rules (`settings.manage`) |
| `GET/PUT /api/v1/pricing/rates`, `PUT/DELETE /api/v1/pricing/rates/{currency}` | Manually maintained exchange rates (`settings.manage`) |
| `POST /api/v1/pricing/preview` | Evaluate the rules or a draft rule for a sample, without saving (`settings.manage`) |
| `POST /api/v1/products/reprice` | Recalculate by ids or filter; `dry_run` returns before/after (`products.write`) |
| `PUT /api/v1/products/{id}/pricing` | Lock a manual price or unlock it (`products.write`) |

Rule semantics, rounding, worked examples and rate maintenance scripts: [pricing.md](pricing.md).
