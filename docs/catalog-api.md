# Catalog API and webhooks

Products imported from marketplaces are reviewed in REA. Once approved, your other systems (your
own storefront, an ERP, a feed generator, scripts) read them through the **catalog API** and can
be notified of every change through **webhooks**. Nothing is pushed to a store by REA itself:
your system pulls the catalog or receives events at an endpoint you register.

- Endpoints: `/api/v1/catalog/...` (read) and `POST /api/v1/imports/products` (submit for review).
- Credentials: **integration keys** (`reai_…`) created under **Settings → Integrations**.
- Machine-readable reference: the OpenAPI document at `/api/v1/openapi.json` (tag `Catalog`); the
  webhook body is the `CatalogEvent` schema, described under `x-webhooks`.

## Authentication

Send an integration key as a bearer token:

```bash
export REA_API='https://rea.example.com/api/v1'
export REA_KEY='reai_…'
curl -fsS -H "Authorization: Bearer $REA_KEY" "$REA_API/catalog/products?limit=1"
```

Administrators (`settings.manage`) create keys in the dashboard; key management needs a browser
session, so an API token can never mint new credentials. A key is shown once, stored as a SHA-256
hash, can have an expiry date, and can be revoked at any time (effective immediately).

| Scope | Allows |
| --- | --- |
| `catalog.read` | `GET /catalog/products`, `/catalog/products/{id}`, `/catalog/products/by-sku/{sku}`, `/catalog/changes` |
| `catalog.read_costs` | Additionally the `sourcing` blocks of products (cost, margin, observed source price, supplier, source URL and domain) and variants (cost and source price), sourcing-only changes, and the `source_domain` filter. Requires `catalog.read`. |
| `imports.write` | `POST /imports/products/check` and `POST /imports/products`: submit products to the import queue for review |
| `orders.write` | Submit and cancel customer orders and read the orders submitted with the same key; see [orders.md](orders.md) |
| `supplier_orders.report` | For your purchasing automation: read confirmed supplier orders of suppliers with automated placement and report placements, failures and shipments; see [orders.md](orders.md) |

**Keys belong to the organization, not to a person.** Their scopes are fixed when they are created
and are never combined with anyone's role. Downgrading or deleting the administrator who created a
key therefore neither widens nor breaks it; the key list keeps showing who created it (or the
e-mail recorded at creation when that user was deleted). When someone with access to keys leaves,
review and rotate the keys they created.

No scope lets a key confirm a supplier order: that is always a person's decision in the dashboard.

**Integration keys reach nothing else.** Requests with a key to any other endpoint (CRM, products
in the dashboard sense, users, settings, audit log, profile, AI, analysis, integrations
management) are refused with `403` before routing, in addition to the normal permission checks.

Users can also call the catalog endpoints with their session or a personal API token when their
role includes `products.read`; they see the sourcing block, as they do in the dashboard.

**Rate limits and audit.** Each key has its own limit (`REA_INTEGRATION_RATE_LIMIT`, default 600
requests per minute; `429 rate_limited` with `Retry-After` when exceeded), on top of the per-IP
limit. Every request is counted per key and day (shown in the dashboard), the first request in
each hour and every request from a new IP address are written to the audit log
(`integration_key.use`), and everything a key changes (imports) is attributed to the key in the
audit log.

## The catalog

A product is **published** when its status is `active` and its review is `approved`. Only
published products appear in the catalog; drafts, products in review and archived products never
do.

```json
{
  "id": "adc19ff6-e403-42de-9aa7-d1d842c97064",
  "version": "1842",
  "sku": "FLASK-1L",
  "title": "Insulated flask 1 L",
  "brand": "Northwind", "model": "", "mpn": "", "gtin": "4006381333931",
  "identifiers": { "asin": "B0C1234567" },
  "category": "Outdoor > Drinkware",
  "availability": "in_stock",
  "pricing": { "price": "29.9", "currency": "EUR", "compare_at_price": "39.9" },
  "primary_image_url": "https://cdn.example.com/flask.jpg",
  "tags": ["Summer sale"],
  "created_at": "2026-10-17T05:27:40Z",
  "updated_at": "2026-10-17T05:28:02Z",
  "description": "Double-wall stainless steel flask.",
  "specifications": { "Capacity": "1 L" },
  "custom_fields": {},
  "variants": [
    { "id": "…", "sku": "FLASK-1L-BLK", "title": "Black", "options": { "Color": "Black" },
      "pricing": { "price": "29.9", "currency": "EUR", "compare_at_price": "39.9" }, "availability": "in_stock", "image_url": null,
      "position": 0, "sourcing": { "cost": "11.2", "cost_currency": "EUR", "source_price": "12.4", "source_currency": "USD" } }
  ],
  "images": [
    { "id": "…", "url": "https://cdn.example.com/flask.jpg", "alt_text": "", "width": null, "height": null, "position": 0, "is_primary": true }
  ],
  "sourcing": {
    "cost": "11.2", "cost_currency": "EUR", "margin_amount": "18.7", "margin_percent": "62.54",
    "source_price": "12.4", "source_currency": "USD",
    "supplier": { "id": "…", "name": "Ningbo Flasks" },
    "source_url": "https://supplier.example/p/1", "canonical_url": "https://supplier.example/p/1", "source_domain": "supplier.example"
  }
}
```

- Money values are decimal strings with a separate ISO 4217 currency. Images list the primary
  image first.
- `version` is the change-feed position of the product's latest change; `updated_at` is when the
  catalog representation last changed (including variants, images and tags).
- `pricing` is the selling price set by your [pricing rules](pricing.md) or entered by hand;
  `compare_at_price` (the "was" price, same currency) is `null` unless a rule or a person set one.
- `sourcing` (on the product and on each variant) is present only with `catalog.read_costs`.
  `source_price` is the marketplace price observed at import, in `source_currency`; `cost` is what
  the product costs you.
- `fields=summary` omits `description`, `specifications`, `custom_fields`, `variants` and `images`.

### Endpoints

| Request | Purpose |
| --- | --- |
| `GET /catalog/products` | Published products ordered by `version` (least recently changed first). Query: `limit` (≤ 200, default 50), `cursor`, `updated_since` (RFC 3339), `tag` (name), `category`, `availability`, `sku` (product or variant SKU), `source_domain` (needs `catalog.read_costs`), `fields`. Response: `items`, `next_cursor` (null on the last page), `has_more`, `change_cursor`. |
| `GET /catalog/products/{id}` | One published product (`404` otherwise). |
| `GET /catalog/products/by-sku/{sku}` | By product SKU, else by variant SKU, case-insensitive. `409 sku_ambiguous` when several products match; list them with `?sku=`. |
| `GET /catalog/changes?since=<cursor>` | The change feed after a cursor, oldest first. Query: `limit` (≤ 500, default 100), `fields` (`full`, `summary` or `none`). Response: `items` (events), `next_cursor`, `has_more`. |

The list and product endpoints return a weak `ETag`; send it back in `If-None-Match` to get `304
Not Modified` without a body when nothing changed. Cursors are opaque strings; store them exactly
as returned.

**Keyset pagination is stable.** Products that exist when you start are returned exactly once.
Products added or changed while you page move to the end of the order (their version grows), so
you see them again later in the same pass instead of missing them.

## Keeping a copy in sync

1. **Initial load.** Page through `GET /catalog/products` with `cursor=<next_cursor>` until
   `has_more` is false. Save `change_cursor` from the **first** page.
2. **Follow the feed.** Call `GET /catalog/changes?since=<saved cursor>` repeatedly (or when a
   webhook arrives). Apply the items in order and save `next_cursor` after each page:
   - `product.unpublished`, `product.deleted`: delete the product (`product_id`) locally;
   - any other event: upsert `product` (if `product` is `null`, the product has left the catalog
     since; a tombstone follows later in the feed).
3. **Resync on 410.** `410 cursor_expired` means your cursor is older than the retained feed
   (`REA_CATALOG_CHANGES_RETENTION_DAYS`, default 90 days); `410 cursor_ahead` means the cursor is
   newer than the feed (for example after a database restore). Run step 1 again.

```bash
cursor=$(curl -fsS -H "Authorization: Bearer $REA_KEY" "$REA_API/catalog/products?limit=200" | jq -r .change_cursor)
# ... page through the products and store them ...
while :; do
  page=$(curl -fsS -H "Authorization: Bearer $REA_KEY" "$REA_API/catalog/changes?since=$cursor&limit=200")
  echo "$page" | jq -c '.items[]' | while read -r ev; do
    case $(echo "$ev" | jq -r .type) in
      product.unpublished|product.deleted) echo "delete $(echo "$ev" | jq -r .product_id)" ;;
      *) echo "upsert $(echo "$ev" | jq -r .product_id)" ;;
    esac
  done
  cursor=$(echo "$page" | jq -r .next_cursor)
  [ "$(echo "$page" | jq -r .has_more)" = true ] || sleep 30
done
```

**Ordering guarantee.** Feed positions follow the order in which the writing transactions
committed, and a position is never filled in later: once you have read up to N, no change below N
will appear afterwards. Each item carries the product's current representation at read time, so
applying events in order converges to the catalog exactly; applying the same event twice is
harmless. A consumer starting from `since=0` (while nothing has been pruned) receives the whole
catalog, because products that were already published when the feature was installed start the
feed as `product.published`.

### Events

| Type | When |
| --- | --- |
| `product.published` | A product entered the catalog (approved in review and active) |
| `product.updated` | Anything else visible changed: title, description, identifiers, specifications, custom fields, category, images, variants, tags (including a renamed tag); for clients that see costs also cost, source price (of the product or a variant), supplier (including a renamed supplier) and source URL |
| `product.price_changed` | The selling price, currency or compare-at price of the product or one of its variants changed |
| `product.availability_changed` | The availability of the product or one of its variants changed |
| `product.unpublished` | The product left the catalog (archived, set back to draft, review reopened or rejected); tombstone |
| `product.deleted` | A published product was deleted or merged into another product; tombstone |

One save can produce several events (for example `price_changed` and `availability_changed`);
each carries the complete product. `changed_fields` names what changed in catalog terms
(`pricing.price`, `pricing.compare_at_price`, `variants.pricing`, `variants.availability`,
`images`, `tags`, `sourcing.cost`, `sourcing.source_price`, `sourcing.variants`, ...). Changes
that only touch sourcing data (including variant costs and source prices) are delivered only to
clients with `catalog.read_costs` (and to webhooks with *include costs*); other clients never see
sourcing field names. Pricing bookkeeping (which rule priced a product, when, and whether its
price is locked) is not part of the catalog, so a reprice that leaves the prices unchanged
produces no change.

Changes are captured by database triggers on products, variants, images, tag links, tag names
and supplier names, inside the writing transaction. Every write path is covered: the dashboard,
the extension, CSV and API imports, bulk actions, merges, AI apply, repricing jobs and direct SQL.
Columns that are not part of the catalog (internal notes, extraction details, `last_checked_at`)
do not produce events.

## Webhooks

Register endpoints under **Settings → Integrations → Webhooks** (administrators only): a name,
an HTTPS URL, the events to receive, whether to include costs, and whether it is enabled. Besides
the product events below, endpoints can subscribe to order events (`order.*`, `supplier_order.*`),
which carry customer personal data and are described in [orders.md](orders.md#webhooks). REA
generates the signing secret, shows it once and stores it encrypted with `REA_SECRET_KEY`
(AES-256-GCM). A new endpoint receives changes made after it was created.

Each event is POSTed as JSON (`Content-Type: application/json`, `User-Agent: REA-Webhooks/1.0`).
The body is exactly the feed item (`CatalogEvent`):

```json
{
  "id": "8cc15df9-927d-43b8-ad81-47b8bc1cfac0",
  "type": "product.published",
  "cursor": "1",
  "occurred_at": "2026-10-17T05:27:58.1Z",
  "product_id": "adc19ff6-e403-42de-9aa7-d1d842c97064",
  "sku": "FLASK-1L",
  "changed_fields": [],
  "product": { "id": "adc19ff6-…", "title": "Insulated flask 1 L", "...": "..." }
}
```

`product` is the representation when the delivery was created (with `sourcing` only for endpoints
with *include costs*) and `null` for tombstones. The **Send test event** button posts a
`webhook.test` event with a sample product and an empty `cursor`.

| Header | Value |
| --- | --- |
| `X-REA-Event-Id` | The event ID (`id` in the body). Identical in the feed, in retries and in redeliveries: deduplicate on it. |
| `X-REA-Event-Type` | The event type |
| `X-REA-Delivery-Id` | This delivery; a redelivery has a new one |
| `X-REA-Timestamp` | Unix seconds when the request was signed |
| `X-REA-Signature` | `v1=<hex HMAC-SHA256(secret, timestamp + "." + raw body)>`; two comma-separated values while a rotated secret is still valid |

### Verifying signatures

Use the raw body bytes, before any JSON parsing, reject timestamps older than a few minutes, and
compare in constant time against every `v1=` value.

Node.js:

```js
import { createHmac, timingSafeEqual } from 'node:crypto'

export function verifyREA(rawBody, headers, secret, toleranceSeconds = 300) {
  const timestamp = headers['x-rea-timestamp']
  const header = headers['x-rea-signature'] ?? ''
  if (!timestamp || Math.abs(Date.now() / 1000 - Number(timestamp)) > toleranceSeconds) return false
  const expected = createHmac('sha256', secret).update(`${timestamp}.`).update(rawBody).digest('hex')
  return header.split(',').some((part) => {
    const value = part.trim().replace(/^v1=/, '')
    return value.length === expected.length && timingSafeEqual(Buffer.from(value), Buffer.from(expected))
  })
}

// Express: app.post('/hooks/rea', express.raw({ type: 'application/json' }), (req, res) => {
//   if (!verifyREA(req.body, req.headers, process.env.REA_WEBHOOK_SECRET)) return res.sendStatus(401)
//   const event = JSON.parse(req.body); ...; res.sendStatus(204) })
```

Go:

```go
func verifyREA(body []byte, timestamp, header, secret string) bool {
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || math.Abs(float64(time.Now().Unix()-ts)) > 300 {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	for _, part := range strings.Split(header, ",") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(part), "v1="); ok && hmac.Equal([]byte(v), []byte(want)) {
			return true
		}
	}
	return false
}
```

The key is the complete secret string including its `whsec_` prefix. **Rotate secret** issues a
new one; the previous secret keeps signing (as a second `v1=` value) for 24 hours, so you can
update the receiver without dropping deliveries.

### Delivery, retries and disabling

- Answer with any `2xx` status within `REA_WEBHOOK_TIMEOUT` (default 10 s). Redirects are not
  followed and count as failures; register the final URL.
- Other answers, timeouts and connection errors are retried with exponential backoff and ±20 %
  jitter: `REA_WEBHOOK_RETRY_BASE` (1 minute) doubling up to `REA_WEBHOOK_RETRY_MAX` (6 hours),
  for at most `REA_WEBHOOK_MAX_ATTEMPTS` (10) attempts, about 8.5 hours in total by default. The
  delivery is then marked failed.
- After `REA_WEBHOOK_DISABLE_AFTER_FAILURES` (20) consecutive failed attempts that have been
  failing for at least `REA_WEBHOOK_DISABLE_FAILING_FOR` (1 hour), the endpoint is disabled
  automatically; the reason (count, time and last error) is shown on the endpoint and written to
  the audit log. Re-enabling it resumes delivery where it stopped, including changes made while
  it was disabled (within the feed retention).
- Deliveries are not ordered across retries. Compare `cursor` with the `version` you already
  hold for that product and ignore older events, or fetch the product when in doubt.
- The **Deliveries** log shows each delivery with status, HTTP status, latency, attempt count, the
  exact payload, every attempt and the first 2 KB of the response (at most 64 KB is read). Any
  real event can be **redelivered** (same event ID, new delivery ID). Finished deliveries are
  kept for `REA_WEBHOOK_LOG_RETENTION_DAYS` (30).

Webhooks are complementary to the feed: if your receiver was down for longer than the retry
window, catch up from the feed with the last cursor you processed.

## Submitting products (imports.write)

A key with `imports.write` can use the same import endpoints as the browser extension:

```bash
curl -fsS -H "Authorization: Bearer $REA_KEY" -H 'Content-Type: application/json' "$REA_API/imports/products/check" \
  -d '{"payload":{"url":"https://supplier.example/p/1","title":"Steel bottle 750 ml","price":"19.90","currency":"EUR"}}'
curl -fsS -H "Authorization: Bearer $REA_KEY" -H 'Content-Type: application/json' "$REA_API/imports/products" \
  -d '{"payload":{"url":"https://supplier.example/p/1","title":"Steel bottle 750 ml","price":"19.90","currency":"EUR"},"tag_names":["erp"]}'
```

Submitted products are drafts pending review; nothing is published until a person approves it.
For integration keys the import source is always `api`, a likely duplicate is parked in the
import queue (`202`) for a person to decide (`decision: update_existing` is refused), suppliers
are not created, and duplicate candidates are returned without their stored records.

## Errors

Errors use the standard envelope `{"error": {"code", "message", "fields"}}`: `401 token_expired`
(invalid, expired or revoked key), `403 forbidden` (missing scope, or an endpoint keys cannot
use), `404`, `409 sku_ambiguous`, `410 cursor_expired` / `cursor_ahead`, `422 validation_failed`,
`429 rate_limited`.

## Security notes

- Integration keys have 256 bits of entropy, are stored as SHA-256 hashes and never logged;
  request logs record the key ID only. Webhook secrets are encrypted at rest and never returned
  after creation or rotation; the audit log redacts them.
- Webhook URLs must use HTTPS. Plain `http://` is accepted only for `localhost` when
  `REA_WEBHOOK_ALLOW_HTTP_LOCALHOST=true` (development).
- Outgoing requests resolve the host and check the address **at connection time**, so DNS
  rebinding cannot reach internal services: private, loopback, link-local, CGNAT, multicast and
  reserved ranges are refused unless `REA_WEBHOOK_ALLOW_PRIVATE_NETWORKS=true`; cloud metadata
  addresses (169.254.169.254, 169.254.170.2, 100.100.100.200, fd00:ec2::254) are always refused.
  Proxies from the environment are ignored, redirects are never followed, every request has a
  timeout and response bodies are capped.
- Only administrators with `settings.manage`, in a browser session, can create keys or webhooks;
  every change (create, revoke, update, rotate, test, redeliver, automatic disable) is audited.
- *Include costs* on a webhook and `catalog.read_costs` on a key expose costs, margins and
  suppliers. Grant them only to systems you trust with that information.
