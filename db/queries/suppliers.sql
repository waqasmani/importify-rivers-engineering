-- name: ListSuppliers :many
SELECT sqlc.embed(s), co.name AS company_name, ps.name AS stage_name, ps.color AS stage_color,
  concat_ws(' ', pc.first_name, pc.last_name)::text AS primary_contact_name,
  (SELECT count(*) FROM supplier_products sp WHERE sp.supplier_id = s.id) AS product_count,
  count(*) OVER () AS total
FROM suppliers s
LEFT JOIN companies co ON co.id = s.company_id
LEFT JOIN pipeline_stages ps ON ps.id = s.pipeline_stage_id
LEFT JOIN contacts pc ON pc.id = s.primary_contact_id
WHERE (sqlc.narg('q')::text IS NULL
       OR rea_search_doc(s.name, s.domain, s.country, s.description) @@ websearch_to_tsquery('simple', sqlc.narg('q'))
       OR s.normalized_name % lower(sqlc.narg('q')) OR s.name ILIKE '%' || sqlc.narg('q') || '%')
  AND (sqlc.narg('supplier_type')::text IS NULL OR s.supplier_type = sqlc.narg('supplier_type'))
  AND (sqlc.narg('company_id')::uuid IS NULL OR s.company_id = sqlc.narg('company_id'))
  AND (sqlc.narg('stage_id')::uuid IS NULL OR s.pipeline_stage_id = sqlc.narg('stage_id'))
  AND (sqlc.narg('country')::text IS NULL OR s.country ILIKE sqlc.narg('country'))
  AND (sqlc.narg('min_rating')::int IS NULL OR s.rating >= sqlc.narg('min_rating'))
  AND (sqlc.narg('owner_id')::uuid IS NULL OR s.owner_id = sqlc.narg('owner_id'))
  AND (sqlc.narg('tag_id')::uuid IS NULL OR EXISTS (
        SELECT 1 FROM record_tags rt WHERE rt.entity_type = 'supplier' AND rt.entity_id = s.id AND rt.tag_id = sqlc.narg('tag_id')))
ORDER BY
  CASE WHEN sqlc.arg(sort)::text = 'name' THEN s.name END ASC,
  CASE WHEN sqlc.arg(sort)::text = '-name' THEN s.name END DESC,
  CASE WHEN sqlc.arg(sort)::text = '-rating' THEN s.rating END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort)::text = 'created' THEN s.created_at END ASC,
  CASE WHEN sqlc.arg(sort)::text = '-created' THEN s.created_at END DESC,
  s.updated_at DESC, s.id
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: GetSupplier :one
SELECT * FROM suppliers WHERE id = $1;

-- name: CreateSupplier :one
INSERT INTO suppliers (name, normalized_name, company_id, supplier_type, website, domain, country, city, address, email,
                       phone, primary_contact_id, pipeline_stage_id, rating, payment_terms, lead_time_days, moq_notes,
                       certifications, description, owner_id, custom_fields, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)
RETURNING *;

-- name: UpdateSupplier :one
UPDATE suppliers SET name = $2, normalized_name = $3, company_id = $4, supplier_type = $5, website = $6, domain = $7,
  country = $8, city = $9, address = $10, email = $11, phone = $12, primary_contact_id = $13, pipeline_stage_id = $14,
  rating = $15, payment_terms = $16, lead_time_days = $17, moq_notes = $18, certifications = $19, description = $20,
  owner_id = $21, custom_fields = $22
WHERE id = $1 RETURNING *;

-- name: SetSupplierStage :one
UPDATE suppliers SET pipeline_stage_id = $2 WHERE id = $1 RETURNING *;

-- name: DeleteSupplier :execrows
DELETE FROM suppliers WHERE id = $1;

-- name: FindSupplierDuplicates :many
SELECT s.*, similarity(s.normalized_name, sqlc.arg(normalized_name)::text)::float8 AS score
FROM suppliers s
WHERE s.id <> coalesce(sqlc.narg('exclude_id')::uuid, '00000000-0000-0000-0000-000000000000'::uuid)
  AND ((sqlc.narg('domain')::text IS NOT NULL AND s.domain = sqlc.narg('domain'))
       OR s.normalized_name % sqlc.arg(normalized_name)::text)
ORDER BY (s.domain IS NOT DISTINCT FROM sqlc.narg('domain')) DESC, score DESC
LIMIT 10;

-- name: FindSupplierByDomain :one
SELECT * FROM suppliers WHERE domain = $1 ORDER BY created_at LIMIT 1;

-- name: SetSupplierStageBulk :execrows
UPDATE suppliers SET pipeline_stage_id = sqlc.narg('stage_id') WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: SetSupplierOwnerBulk :execrows
UPDATE suppliers SET owner_id = sqlc.narg('owner_id') WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: DeleteSuppliersBulk :execrows
DELETE FROM suppliers WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: ReassignSupplierReferences :exec
WITH c1 AS (UPDATE contacts SET supplier_id = sqlc.arg(target_id) WHERE supplier_id = ANY(sqlc.arg(source_ids)::uuid[])),
     c2 AS (UPDATE opportunities SET supplier_id = sqlc.arg(target_id) WHERE supplier_id = ANY(sqlc.arg(source_ids)::uuid[])),
     c3 AS (UPDATE products SET supplier_id = sqlc.arg(target_id) WHERE supplier_id = ANY(sqlc.arg(source_ids)::uuid[])),
     -- Move supplier-product links that the target does not already have.
     c4 AS (UPDATE supplier_products sp SET supplier_id = sqlc.arg(target_id)
            WHERE sp.supplier_id = ANY(sqlc.arg(source_ids)::uuid[])
              AND NOT EXISTS (SELECT 1 FROM supplier_products t WHERE t.supplier_id = sqlc.arg(target_id) AND t.product_id = sp.product_id))
SELECT 1;

-- name: ListSupplierProducts :many
SELECT sp.*, p.title AS product_title, p.sku AS product_sku, p.status AS product_status
FROM supplier_products sp JOIN products p ON p.id = sp.product_id
WHERE sp.supplier_id = $1
ORDER BY sp.is_preferred DESC, p.title
LIMIT 500;

-- name: ListProductSuppliers :many
SELECT sp.*, s.name AS supplier_name, s.country AS supplier_country, s.rating AS supplier_rating
FROM supplier_products sp JOIN suppliers s ON s.id = sp.supplier_id
WHERE sp.product_id = $1
ORDER BY sp.is_preferred DESC, s.name;

-- name: UpsertSupplierProduct :one
INSERT INTO supplier_products (supplier_id, product_id, supplier_sku, cost, currency, moq, lead_time_days, is_preferred, notes)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (supplier_id, product_id) DO UPDATE SET supplier_sku = EXCLUDED.supplier_sku, cost = EXCLUDED.cost,
  currency = EXCLUDED.currency, moq = EXCLUDED.moq, lead_time_days = EXCLUDED.lead_time_days,
  is_preferred = EXCLUDED.is_preferred, notes = EXCLUDED.notes
RETURNING *;

-- name: DeleteSupplierProduct :execrows
DELETE FROM supplier_products WHERE supplier_id = $1 AND product_id = $2;
