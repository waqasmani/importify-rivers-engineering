-- name: ListProducts :many
SELECT sqlc.embed(p), s.name AS supplier_name, pr.name AS pricing_rule_name,
  coalesce((SELECT pi.url FROM product_images pi WHERE pi.product_id = p.id ORDER BY pi.is_primary DESC, pi.position LIMIT 1), '')::text AS primary_image_url,
  (SELECT count(*) FROM product_variants pv WHERE pv.product_id = p.id) AS variant_count,
  count(*) OVER () AS total
FROM products p LEFT JOIN suppliers s ON s.id = p.supplier_id LEFT JOIN pricing_rules pr ON pr.id = p.pricing_rule_id
WHERE (sqlc.narg('q')::text IS NULL
       OR rea_search_doc(p.title, p.brand, p.sku, p.description) @@ websearch_to_tsquery('simple', sqlc.narg('q'))
       OR p.normalized_title % lower(sqlc.narg('q')) OR p.title ILIKE '%' || sqlc.narg('q') || '%' OR p.sku ILIKE sqlc.narg('q'))
  AND (sqlc.narg('status')::text IS NULL OR p.status = sqlc.narg('status'))
  AND (sqlc.narg('review_status')::text IS NULL OR p.review_status = sqlc.narg('review_status'))
  AND (sqlc.narg('supplier_id')::uuid IS NULL OR p.supplier_id = sqlc.narg('supplier_id')
       OR EXISTS (SELECT 1 FROM supplier_products sp WHERE sp.product_id = p.id AND sp.supplier_id = sqlc.narg('supplier_id')))
  AND (sqlc.narg('source_domain')::text IS NULL OR p.source_domain = sqlc.narg('source_domain'))
  AND (sqlc.narg('category')::text IS NULL OR p.category ILIKE sqlc.narg('category'))
  AND (sqlc.narg('availability')::text IS NULL OR p.availability = sqlc.narg('availability'))
  AND (sqlc.narg('tag_id')::uuid IS NULL OR EXISTS (
        SELECT 1 FROM record_tags rt WHERE rt.entity_type = 'product' AND rt.entity_id = p.id AND rt.tag_id = sqlc.narg('tag_id')))
ORDER BY
  CASE WHEN sqlc.arg(sort)::text = 'title' THEN p.title END ASC,
  CASE WHEN sqlc.arg(sort)::text = '-price' THEN p.price END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort)::text = 'price' THEN p.price END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort)::text = '-margin' THEN p.margin_percent END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort)::text = '-created' THEN p.created_at END DESC,
  CASE WHEN sqlc.arg(sort)::text = 'checked' THEN p.last_checked_at END ASC,
  p.updated_at DESC, p.id
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: GetProduct :one
SELECT * FROM products WHERE id = $1;

-- name: CreateProduct :one
INSERT INTO products (original_title, title, normalized_title, source_url, canonical_url, source_domain, supplier_id,
  supplier_contact_id, company_id, brand, sku, model, mpn, gtin, identifiers, description, specifications, cost,
  cost_currency, price, price_currency, availability, category, status, review_status, extraction_method,
  extraction_confidence, captured_page_id, source_evidence, internal_notes, custom_fields, created_by, source_price,
  source_currency, cost_manual, compare_at_price, price_locked)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23,
  $24, $25, $26, $27, $28, $29, $30, $31, $32, $33, $34, $35, $36, $37)
RETURNING *;

-- name: UpdateProduct :one
UPDATE products SET title = $2, normalized_title = $3, source_url = $4, canonical_url = $5, source_domain = $6,
  supplier_id = $7, supplier_contact_id = $8, company_id = $9, brand = $10, sku = $11, model = $12, mpn = $13,
  gtin = $14, identifiers = $15, description = $16, specifications = $17, cost = $18, cost_currency = $19, price = $20,
  price_currency = $21, availability = $22, category = $23, status = $24, internal_notes = $25, custom_fields = $26,
  cost_manual = $27, compare_at_price = $28, price_locked = $29,
  pricing_rule_id = CASE WHEN $29 THEN NULL ELSE pricing_rule_id END
WHERE id = $1 RETURNING *;

-- name: RefreshProductFromCapture :one
UPDATE products SET original_title = $2, description = CASE WHEN description = '' THEN $3 ELSE description END,
  specifications = specifications || $4, source_price = coalesce(sqlc.narg('source_price'), source_price),
  source_currency = coalesce(sqlc.narg('source_currency'), source_currency), availability = $5,
  extraction_method = $6, extraction_confidence = $7, captured_page_id = coalesce(sqlc.narg('captured_page_id'), captured_page_id),
  source_evidence = source_evidence || $8, last_checked_at = now(),
  brand = CASE WHEN brand = '' THEN $9 ELSE brand END, sku = CASE WHEN sku = '' THEN $10 ELSE sku END,
  gtin = coalesce(gtin, sqlc.narg('gtin')), mpn = CASE WHEN mpn = '' THEN $11 ELSE mpn END,
  identifiers = $12 || identifiers
WHERE id = $1 RETURNING *;

-- name: SetProductReview :one
UPDATE products SET review_status = $2, status = $3 WHERE id = $1 RETURNING *;

-- name: SetProductSupplier :one
UPDATE products SET supplier_id = $2, supplier_contact_id = $3 WHERE id = $1 RETURNING *;

-- name: SetProductEnrichment :one
UPDATE products SET title = $2, normalized_title = $3, category = $4, description = $5, specifications = $6, brand = $7
WHERE id = $1 RETURNING *;

-- name: DeleteProduct :execrows
DELETE FROM products WHERE id = $1;

-- name: DeleteProductsBulk :execrows
DELETE FROM products WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: SetProductReviewBulk :execrows
UPDATE products SET review_status = sqlc.arg(review_status),
  status = CASE WHEN sqlc.arg(review_status) = 'approved' AND status = 'draft' THEN 'active'
                WHEN sqlc.arg(review_status) = 'rejected' THEN 'archived' ELSE status END
WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: SetProductStatusBulk :execrows
UPDATE products SET status = sqlc.arg(status) WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: SetProductSupplierBulk :execrows
UPDATE products SET supplier_id = sqlc.narg('supplier_id') WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: FindProductByCanonicalURL :many
SELECT * FROM products WHERE canonical_url = $1 OR source_url = $1 LIMIT 5;

-- name: FindProductByGTIN :many
SELECT * FROM products WHERE gtin = $1 LIMIT 5;

-- name: FindProductByDomainSKU :many
SELECT * FROM products WHERE source_domain = $1 AND lower(sku) = lower(sqlc.arg(sku)::text) AND sku <> '' LIMIT 5;

-- name: FindProductByBrandMPN :many
SELECT * FROM products WHERE lower(brand) = lower(sqlc.arg(brand)::text) AND lower(mpn) = lower(sqlc.arg(mpn)::text) AND mpn <> '' LIMIT 5;

-- name: FindProductsBySimilarTitle :many
SELECT p.*, similarity(p.normalized_title, sqlc.arg(normalized_title)::text)::float8 AS score
FROM products p
WHERE p.normalized_title % sqlc.arg(normalized_title)::text
  AND p.id <> coalesce(sqlc.narg('exclude_id')::uuid, '00000000-0000-0000-0000-000000000000'::uuid)
ORDER BY score DESC
LIMIT 10;

-- name: ListProductsByIDs :many
SELECT * FROM products WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: ListVariants :many
SELECT * FROM product_variants WHERE product_id = $1 ORDER BY position, title;

-- name: CreateVariant :one
INSERT INTO product_variants (product_id, title, sku, options, price, currency, availability, image_url, position,
  source_price, source_currency, cost, cost_currency, cost_manual, compare_at_price, price_locked)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16) RETURNING *;

-- name: UpdateVariant :one
UPDATE product_variants SET title = $3, sku = $4, options = $5, price = $6, currency = $7, availability = $8,
  image_url = $9, position = $10, cost = $11, cost_currency = $12, cost_manual = $13, compare_at_price = $14,
  price_locked = $15, pricing_rule_id = CASE WHEN $15 THEN NULL ELSE pricing_rule_id END
WHERE id = $1 AND product_id = $2 RETURNING *;

-- name: RefreshVariantFromCapture :one
UPDATE product_variants SET title = $3, sku = $4, options = $5, source_price = coalesce(sqlc.narg('source_price'), source_price),
  source_currency = coalesce(sqlc.narg('source_currency'), source_currency), availability = $6, image_url = $7
WHERE id = $1 AND product_id = $2 RETURNING *;

-- name: GetVariant :one
SELECT * FROM product_variants WHERE id = $1 AND product_id = $2;

-- name: DeleteVariant :execrows
DELETE FROM product_variants WHERE id = $1 AND product_id = $2;

-- name: DeleteProductVariants :exec
DELETE FROM product_variants WHERE product_id = $1;

-- name: ListImages :many
SELECT * FROM product_images WHERE product_id = $1 ORDER BY is_primary DESC, position;

-- name: UpsertImage :one
INSERT INTO product_images (product_id, url, source_page, alt_text, width, height, position, is_primary)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (product_id, url) DO UPDATE SET alt_text = EXCLUDED.alt_text, position = EXCLUDED.position
RETURNING *;

-- name: DeleteImage :execrows
DELETE FROM product_images WHERE id = $1 AND product_id = $2;

-- name: SetPrimaryImage :exec
UPDATE product_images SET is_primary = (id = sqlc.arg(image_id)::uuid) WHERE product_id = sqlc.arg(product_id)::uuid;

-- name: MoveProductChildren :exec
WITH v AS (UPDATE product_variants SET product_id = sqlc.arg(target_id) WHERE product_id = ANY(sqlc.arg(source_ids)::uuid[])),
     i AS (INSERT INTO product_images (product_id, url, source_page, alt_text, width, height, position, is_primary)
           SELECT sqlc.arg(target_id), url, source_page, alt_text, width, height, position + 1000, false
           FROM product_images WHERE product_id = ANY(sqlc.arg(source_ids)::uuid[])
           ON CONFLICT (product_id, url) DO NOTHING),
     sp AS (INSERT INTO supplier_products (supplier_id, product_id, supplier_sku, cost, currency, moq, lead_time_days, is_preferred, notes)
            SELECT supplier_id, sqlc.arg(target_id), supplier_sku, cost, currency, moq, lead_time_days, false, notes
            FROM supplier_products WHERE product_id = ANY(sqlc.arg(source_ids)::uuid[])
            ON CONFLICT (supplier_id, product_id) DO NOTHING),
     op AS (INSERT INTO opportunity_products (opportunity_id, product_id, quantity, unit_price, currency, notes)
            SELECT opportunity_id, sqlc.arg(target_id), quantity, unit_price, currency, notes
            FROM opportunity_products WHERE product_id = ANY(sqlc.arg(source_ids)::uuid[])
            ON CONFLICT (opportunity_id, product_id) DO NOTHING)
UPDATE product_imports SET product_id = sqlc.arg(target_id) WHERE product_id = ANY(sqlc.arg(source_ids)::uuid[]);

-- name: ProductDomains :many
SELECT source_domain, count(*) AS product_count FROM products WHERE source_domain <> '' GROUP BY source_domain ORDER BY 2 DESC LIMIT 100;

-- name: ProductCategories :many
SELECT category, count(*) AS product_count FROM products WHERE category <> '' GROUP BY category ORDER BY 2 DESC LIMIT 200;
