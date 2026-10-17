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
| Integration keys | Your storefront, ERP and other systems | Created by administrators under Settings → Integrations. Send `Authorization: Bearer reai_…`. Organization-owned, scoped (`catalog.read`, `catalog.read_costs`, `imports.write`) and limited to the catalog and product import endpoints. See [catalog-api.md](catalog-api.md). |

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

## Example

```bash
BASE=http://localhost
TOKEN=reap_your_token
curl -fsS -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/products?review_status=pending&limit=20"
curl -fsS -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"payload":{"url":"https://shop.example/p/1","title":"Steel bottle 750 ml","price":"19.90","currency":"EUR"},"source":"api","decision":"","create_supplier":false,"tag_names":[]}' \
  "$BASE/api/v1/imports/products/check"
```
