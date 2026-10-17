# Pricing

REA turns the supplier price it observes on a marketplace into a selling price with ordered
pricing rules. Prices only change REA's own records: nothing is published to a store, ordered or
sent anywhere. Other systems read the prices through the REST API.

## Three prices

| Field | Meaning | Set by |
| --- | --- | --- |
| `source_price`, `source_currency` | The supplier's price observed at import (the marketplace price). | Every import of the page (create, update existing, resolve, CSV `source_price`). Never edited by hand. |
| `cost`, `cost_currency` | What the product costs you. | Derived from the source price, converted into the base currency when a rate exists — unless a person entered a cost (`cost_manual: true`). |
| `price`, `price_currency`, `compare_at_price` | The selling price and the crossed-out list price. | The first matching pricing rule (`pricing_rule_id`, `priced_at`) — unless a person locked a manual price (`price_locked: true`). |

`margin_amount` and `margin_percent` (of the selling price) are calculated by the database when
cost and price share a currency. Variants have the same fields; a variant without a cost of its
own is priced from the product's cost.

**Base currency** is the default currency in Settings → General (`imports.default_currency`). Rule
amounts, cost ranges and computed selling prices are in the base currency.

## When prices are calculated

- **Imports** — a new product, *update existing* on a re-import, resolving a parked duplicate and
  CSV imports all store the observed price as the source price, re-derive the cost and apply the
  rules (after tags are attached, so tag conditions work). Import review screens show the price
  the rules give before anything is stored (`pricing` in `POST /imports/products/check` and
  `GET /imports/{id}`).
- **Edits** — creating a product, or editing anything the rules depend on (cost, category,
  supplier, source domain, tags, the lock), reprices the product and its variants. Adding or
  editing a variant reprices that variant.
- **On demand** — *Recalculate* on a product, or *Reprice* on the products list (selected
  products or everything the current filters match) with a dry run first.

Changing rules or exchange rates does **not** touch stored products by itself; run a reprice so
you can review the before/after first. Locked prices are never changed by rules. Repricing also
re-derives non-manual costs from source prices with the current rates, including for locked
products (the lock covers the selling and compare-at price, not the cost).

### Manual costs and manual prices

- Entering a cost that differs from the derived one makes it manual; clearing the cost makes REA
  derive it from the source price again.
- `price_locked: true` keeps `price` and `compare_at_price` exactly as entered. Lock or unlock
  with `PUT /api/v1/products/{id}/pricing`; unlocking immediately recalculates the price.
- In `POST`/`PUT /api/v1/products` and the variant endpoints, `price_locked` is optional. When it
  is omitted, sending a price that differs from the stored one locks it (a manual price), and
  sending the stored price unchanged keeps the current lock state. `price_locked: false`
  hands the price to the rules.
- CSV imports: a `source_price` column is priced by the rules; a `price` (selling price) column
  is stored as a locked manual price; a `cost` column is a manual cost.

## Rules

Rules are evaluated in priority order (lowest first; *Settings → Pricing* lets you drag them, use
the arrow keys on the handle, or the up/down buttons). **The first enabled rule whose conditions
all match sets the price.** The **default rule** is always evaluated last, has no conditions and
cannot be deleted; it starts as *selling price = cost* so upgrading changes no numbers until you
add a markup. If you disable it, products no rule matches keep their current price
(`no_rule`).

### Conditions

Every condition that is set must hold; lists match any of their entries. A rule without
conditions matches every product.

| Condition | Matches when |
| --- | --- |
| `source_domains` | The product's source domain equals an entry or is a subdomain of it (`aliexpress.com` matches `www.aliexpress.com`; `*.aliexpress.com` is the same). |
| `supplier_ids` | The product's supplier is one of them. |
| `category_contains` | The category contains the text (case-insensitive). |
| `tag_ids` | The product has any of the tags. |
| `min_cost`, `max_cost` | `min_cost ≤ cost < max_cost`, cost converted into the base currency. The upper bound is exclusive so tiers such as 0–10, 10–50, 50+ never overlap. |

### Actions, in order

1. `fixed_amount` when `fixed_amount_before_markup` is true (e.g. shipping you want marked up).
2. `multiplier` (e.g. `2.5`).
3. `markup_percent` (e.g. `50` → × 1.5).
4. `fixed_amount` when added after the markup (e.g. handling passed through at cost).
5. Minimums: `min_price`, `min_margin_amount` (price ≥ cost + amount) and `min_margin_percent`
   (margin as a share of the selling price: price ≥ cost / (1 − m/100)). The price is raised to
   the highest of them.
6. Rounding (below). Rounding never ends below a minimum: if it would, the next allowed price up
   is used.
7. `compare_at_mode`: `multiplier` (price × value, value > 1) or `percent` (price × (1 + value/100)),
   rounded the same way and always above the selling price.

All arithmetic uses exact decimals (`shopspring/decimal`); conversions keep 4 decimal places and
selling prices are whole minor units of the currency (2 decimals for USD/EUR, 0 for JPY, 3 for
KWD).

### Rounding

- `none` — round half up to the currency's minor unit: 14.2857 → 14.29.
- `ending` with `rounding_ending` (`0.99`, `0.95`, `0.49`, `0.00`, or any fraction below 1) — the
  **nearest** price with that ending; ties go up; never zero or negative.
- `step` with `rounding_step` (e.g. `0.05`, `0.5`, `5`) — the nearest multiple; ties go up; at least
  one step. Steps finer than the currency's minor unit are aligned to it.

| Rule | Raw price | Result | Why |
| --- | --- | --- | --- |
| ending .99 | 9.995 | **9.99** | half a cent above 9.99 — it does not jump to 10.99 |
| ending .99 | 10.00 | 9.99 | 9.99 is 0.01 away, 10.99 is 0.99 away |
| ending .99 | 10.49 | 10.99 | exactly halfway between 9.99 and 10.99: ties go up |
| ending .99 | 12.20 | 11.99 | nearest |
| ending .95 | 23.40 | 22.95 | nearest |
| ending .49 | 0.30 | 0.49 | never rounds to zero or below |
| ending .00 | 12.50 | 13.00 | tie goes up |
| step 0.05 | 12.375 | 12.40 | tie goes up |
| step 5 | 22.40 | 20 | nearest |
| step 5 | 0.30 | 5 | at least one step |
| ending .99 for JPY | 1333 | 1333 | JPY has no minor unit: whole yen, with a warning |

### Worked examples

**Marketplace markup with conversion.** Base USD, rate 1 EUR = 1.10 USD. Rule *Shop markup*:
domain `shop.example.com`, markup 50 %, ending .99, compare-at 30 % above.

```
source price  20.00 EUR → cost 22.00 USD (20 × 1.10)
+ 50 % markup              33.00
nearest .99                32.99   selling price
compare-at 32.99 × 1.30 = 42.887 → nearest .99 = 42.99
margin 10.99 (33.31 %)
```

A variant observed at 22.00 EUR costs 24.20 USD → 36.30 → **35.99**.

**Fixed amounts.** Cost 10, multiplier 2, fixed 3: after the markup 10 × 2 + 3 = **23**; before
the markup (10 + 3) × 2 = **26**.

**Minimum margin.** Cost 10, multiplier 1.2 = 12, minimum margin 30 %: 10 / 0.7 = 14.2857 →
**14.29** (margin 30.02 %). With ending .99 the nearest ending is 13.99, below the minimum, so the
price becomes **14.99**.

**Minimum price.** Cost 2, markup 50 % = 3, minimum price 9.99 → **9.99**.

**Compare-at stays above the price.** Price 9.99, compare-at × 1.05 = 10.49 rounds back to 9.99,
so the compare-at price becomes the next ending up, **10.99**.

**Tiers by cost.** Rules ordered *Cheap* (`max_cost` 10, × 3), *Mid* (`min_cost` 10, `max_cost` 50,
× 2), default (+ 40 %): a cost of 9.99 uses *Cheap*, exactly 10 uses *Mid*, 50 falls through to
the default rule.

## Exchange rates

Rates are maintained by hand under *Settings → Pricing → Exchange rates* — REA never fetches
rates from the internet. A rate says how many base-currency units one unit of the currency is
worth (`1 EUR = 1.0882 USD`). Rates are stored per base currency: if you change the base currency,
the old rates are listed separately and not used until you switch back.

Without a rate the cost stays in the source currency, no selling price is calculated and the
import (and the product page) say so: *No exchange rate from GBP to USD is recorded*. Add the rate
and reprice.

Operators can keep rates current from a script or cron job with a personal API token that has the
`settings.manage` scope (Profile → API tokens):

```bash
BASE=https://rea.example.com
TOKEN=reap_your_token
# One currency
curl -fsS -X PUT -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"rate":"1.0882","note":"ECB 2026-10-16"}' "$BASE/api/v1/pricing/rates/EUR"
# Several at once (one transaction)
curl -fsS -X PUT -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"rates":[{"currency":"EUR","rate":"1.0882"},{"currency":"CNY","rate":"0.1389"},{"currency":"GBP","rate":"1.27"}]}' \
  "$BASE/api/v1/pricing/rates"
# Then preview the new costs and prices (the token also needs the products.write scope)
curl -fsS -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"filter":{"source_domain":"aliexpress.com"},"dry_run":true}' "$BASE/api/v1/products/reprice"
```

## API

| Endpoint | Permission | Purpose |
| --- | --- | --- |
| `GET /api/v1/pricing/rules` | settings.manage | Rules in evaluation order with supplier/tag names and how many products each priced |
| `POST /api/v1/pricing/rules` | settings.manage | Create a rule (evaluated last unless `priority` is given) |
| `GET`, `PUT`, `DELETE /api/v1/pricing/rules/{id}` | settings.manage | Read, replace, delete (the default rule: 409 `default_rule`) |
| `POST /api/v1/pricing/rules/reorder` | settings.manage | `{"ids": [...]}` — every non-default rule exactly once |
| `GET /api/v1/pricing/rates` | settings.manage | Rates for the base currency (`items`) and for other bases (`other_bases`) |
| `PUT /api/v1/pricing/rates/{currency}` | settings.manage | Create or replace one rate |
| `PUT /api/v1/pricing/rates` | settings.manage | Create or replace up to 200 rates |
| `DELETE /api/v1/pricing/rates/{currency}` | settings.manage | Delete a rate (`?base=GBP` for a rate against another base) |
| `POST /api/v1/pricing/preview` | settings.manage | Evaluate the saved rules — or a draft `rule` — for a sample; nothing is saved |
| `POST /api/v1/products/reprice` | products.write | Recalculate by `ids` (≤ 1000) or `filter` (≤ 1000 matches); `dry_run` returns before/after without saving |
| `PUT /api/v1/products/{id}/pricing` | products.write | Lock a manual price or unlock it |

Create a rule:

```bash
curl -fsS -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{
  "name": "AliExpress", "source_domains": ["aliexpress.com"],
  "multiplier": "2", "fixed_amount": "1.50", "min_margin_percent": "30",
  "rounding": "ending", "rounding_ending": "0.99",
  "compare_at_mode": "multiplier", "compare_at_value": "1.3"
}' "$BASE/api/v1/pricing/rules"
```

Preview the saved rules, or a draft rule before saving it (the draft is applied whatever its
conditions; `conditions_met` and `unmet_conditions` say whether it would match):

```bash
curl -fsS -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"cost":"10","currency":"EUR","source_domain":"www.aliexpress.com"}' "$BASE/api/v1/pricing/preview"
curl -fsS -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"cost":"4","rule":{"name":"Draft","max_cost":"5","multiplier":"3","min_margin_amount":"10"}}' "$BASE/api/v1/pricing/preview"
```

The evaluation lists every step with its running amount. For a rule with multiplier 2, ending .99
and compare-at × 1.3, and a rate of 1 EUR = 1.09 USD:

```json
{
  "status": "priced", "currency": "USD", "cost": "10", "cost_currency": "EUR", "rate": "1.09", "base_cost": "10.9",
  "rule": {"id": "…", "name": "AliExpress", "is_default": false},
  "price": "21.99", "compare_at_price": "28.99", "margin_amount": "11.09", "margin_percent": "50.43",
  "steps": [
    {"kind": "conversion", "label": "10 EUR converted at 1 EUR = 1.09 USD", "amount": "10.9"},
    {"kind": "cost", "label": "Cost", "amount": "10.9"},
    {"kind": "multiplier", "label": "× 2 multiplier", "amount": "21.8"},
    {"kind": "rounding", "label": "Rounded to the nearest price ending in .99", "amount": "21.99"},
    {"kind": "price", "label": "Selling price", "amount": "21.99"},
    {"kind": "compare_at", "label": "Compare-at price: selling price × 1.3", "amount": "28.99"}
  ],
  "warnings": [], "message": ""
}
```

Bulk reprice — dry run, then apply:

```bash
curl -fsS -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"ids":["<product-id>","<product-id>"],"dry_run":true}' "$BASE/api/v1/products/reprice"
curl -fsS -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"ids":["<product-id>","<product-id>"],"dry_run":false}' "$BASE/api/v1/products/reprice"
```

The result has a `summary` (`products`, `changed`, `unchanged`, `locked`, `not_priced`,
`variants`, `variants_changed`) and per product `status` (`repriced`, `locked`, `no_cost`,
`no_rate`, `no_rule`), `changed`, `before`/`after` snapshots (cost, price, compare-at, rule) and
the same for each variant.

Lock and unlock a manual price:

```bash
curl -fsS -X PUT -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"price_locked":true,"price":"39.00","price_currency":"USD","compare_at_price":"49.00"}' "$BASE/api/v1/products/<id>/pricing"
curl -fsS -X PUT -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"price_locked":false}' "$BASE/api/v1/products/<id>/pricing"
```

Product responses carry the stored values (`source_price`, `cost`, `cost_manual`, `price`,
`compare_at_price`, `price_locked`, `pricing_rule_id`, `pricing_rule_name`, `priced_at`,
`margin_amount`, `margin_percent`); `GET /api/v1/products/{id}` adds `pricing`, the breakdown under
the current rules for the product and each variant, with `changed: true` when recalculating now
would change the stored values.

## Audit trail

`pricing_rule.create|update|delete|reorder`, `currency_rate.update|delete`, `product.reprice`
(before/after, reason: import, product edited, bulk reprice, …), `product.reprice_bulk`,
`product.price_lock` and `product.price_unlock` are written to the audit log; product events
appear in the product's activity timeline.

## Upgrading

Migration `00006_pricing` copies every existing `price` into `source_price` (prices stored so far
were observed marketplace prices), leaves `price` unchanged and unlocked, and keeps existing costs
as manual costs. Nothing is repriced until a product is imported again, edited, or repriced; run a
dry-run reprice to see what the rules would change.
