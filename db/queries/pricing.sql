-- name: ListPricingRules :many
SELECT sqlc.embed(r), (SELECT count(*) FROM products p WHERE p.pricing_rule_id = r.id) AS product_count
FROM pricing_rules r
ORDER BY r.is_default, r.priority, r.created_at, r.id;

-- name: ListAllPricingRules :many
SELECT * FROM pricing_rules ORDER BY is_default, priority, created_at, id;

-- name: GetPricingRule :one
SELECT * FROM pricing_rules WHERE id = $1;

-- name: CountProductsByPricingRule :one
SELECT count(*) FROM products WHERE pricing_rule_id = $1;

-- name: NextPricingRulePriority :one
SELECT (coalesce(max(priority), 0) + 10)::integer FROM pricing_rules WHERE NOT is_default;

-- name: CreatePricingRule :one
INSERT INTO pricing_rules (name, description, enabled, priority, source_domains, supplier_ids, category_contains, tag_ids,
  min_cost, max_cost, markup_percent, multiplier, fixed_amount, fixed_amount_before_markup, min_price, min_margin_percent,
  min_margin_amount, rounding, rounding_ending, rounding_step, compare_at_mode, compare_at_value, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23)
RETURNING *;

-- name: UpdatePricingRule :one
UPDATE pricing_rules SET name = $2, description = $3, enabled = $4, priority = $5, source_domains = $6, supplier_ids = $7,
  category_contains = $8, tag_ids = $9, min_cost = $10, max_cost = $11, markup_percent = $12, multiplier = $13,
  fixed_amount = $14, fixed_amount_before_markup = $15, min_price = $16, min_margin_percent = $17, min_margin_amount = $18,
  rounding = $19, rounding_ending = $20, rounding_step = $21, compare_at_mode = $22, compare_at_value = $23
WHERE id = $1 RETURNING *;

-- name: SetPricingRulePriority :execrows
UPDATE pricing_rules SET priority = $2 WHERE id = $1 AND NOT is_default;

-- name: DeletePricingRule :execrows
DELETE FROM pricing_rules WHERE id = $1 AND NOT is_default;

-- name: ListCurrencyRates :many
SELECT * FROM currency_rates ORDER BY base_currency, currency;

-- name: ListCurrencyRatesForBase :many
SELECT * FROM currency_rates WHERE base_currency = $1 ORDER BY currency;

-- name: GetCurrencyRate :one
SELECT * FROM currency_rates WHERE base_currency = $1 AND currency = $2;

-- name: UpsertCurrencyRate :one
INSERT INTO currency_rates (base_currency, currency, rate, note, updated_by)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (base_currency, currency) DO UPDATE SET rate = EXCLUDED.rate, note = EXCLUDED.note, updated_by = EXCLUDED.updated_by
RETURNING *;

-- name: DeleteCurrencyRate :execrows
DELETE FROM currency_rates WHERE base_currency = $1 AND currency = $2;

-- name: SetProductPricing :one
UPDATE products SET cost = $2, cost_currency = $3, price = $4, price_currency = $5, compare_at_price = $6,
  price_locked = $7, pricing_rule_id = $8, priced_at = $9
WHERE id = $1 RETURNING *;

-- name: SetVariantPricing :one
UPDATE product_variants SET cost = $3, cost_currency = $4, price = $5, currency = $6, compare_at_price = $7,
  price_locked = $8, pricing_rule_id = $9, priced_at = $10
WHERE id = $1 AND product_id = $2 RETURNING *;

-- name: ListVariantsForProducts :many
SELECT * FROM product_variants WHERE product_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY product_id, position, title;

-- name: SupplierNamesByIDs :many
SELECT id, name FROM suppliers WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: TagsByIDs :many
SELECT id, name, color FROM tags WHERE id = ANY(sqlc.arg(ids)::uuid[]);
