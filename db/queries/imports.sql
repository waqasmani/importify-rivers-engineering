-- name: CreateProductImport :one
INSERT INTO product_imports (source, status, decision, payload, normalized, warnings, error, duplicate_candidates,
  product_id, duplicate_of_id, captured_page_id, device_id, created_by, processed_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING *;

-- name: GetProductImport :one
SELECT * FROM product_imports WHERE id = $1;

-- name: ListProductImports :many
SELECT sqlc.embed(i), p.title AS product_title, u.display_name AS created_by_name, count(*) OVER () AS total
FROM product_imports i
LEFT JOIN products p ON p.id = i.product_id
LEFT JOIN users u ON u.id = i.created_by
WHERE (sqlc.narg('status')::text IS NULL OR i.status = sqlc.narg('status'))
  AND (sqlc.narg('source')::text IS NULL OR i.source = sqlc.narg('source'))
  AND (sqlc.narg('created_by')::uuid IS NULL OR i.created_by = sqlc.narg('created_by'))
  AND (sqlc.narg('device_id')::uuid IS NULL OR i.device_id = sqlc.narg('device_id'))
ORDER BY i.created_at DESC, i.id
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: ResolveProductImport :one
UPDATE product_imports SET status = $2, decision = $3, product_id = $4, duplicate_of_id = $5, normalized = $6,
  error = $7, reviewed_by = $8, processed_at = now()
WHERE id = $1 RETURNING *;

-- name: CreateCapturedPage :one
INSERT INTO captured_pages (url, canonical_url, domain, title, description, page_type, adapter_key, extraction_method,
  structured_data, extracted, selections, warnings, html_snapshot, content_sha256, captured_by, device_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
RETURNING *;

-- name: GetCapturedPage :one
SELECT * FROM captured_pages WHERE id = $1;

-- name: ListCapturedPages :many
SELECT id, url, canonical_url, domain, title, description, page_type, adapter_key, extraction_method, warnings,
  (html_snapshot IS NOT NULL)::boolean AS has_snapshot, jsonb_array_length(selections) AS selection_count,
  captured_by, device_id, captured_at, count(*) OVER () AS total
FROM captured_pages
WHERE (sqlc.narg('q')::text IS NULL
       OR rea_search_doc(title, description, domain, url) @@ websearch_to_tsquery('simple', sqlc.narg('q'))
       OR title ILIKE '%' || sqlc.narg('q') || '%')
  AND (sqlc.narg('domain')::text IS NULL OR domain = sqlc.narg('domain'))
  AND (sqlc.narg('page_type')::text IS NULL OR page_type = sqlc.narg('page_type'))
  AND (sqlc.narg('device_id')::uuid IS NULL OR device_id = sqlc.narg('device_id'))
  AND (sqlc.narg('captured_by')::uuid IS NULL OR captured_by = sqlc.narg('captured_by'))
ORDER BY captured_at DESC, id
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: DeleteCapturedPage :execrows
DELETE FROM captured_pages WHERE id = $1;

-- name: PurgeCapturedPagesBefore :execrows
DELETE FROM captured_pages WHERE captured_at < $1;

-- name: ListSourceAdapters :many
SELECT * FROM source_adapters ORDER BY priority, key;

-- name: ListEnabledSourceAdapters :many
SELECT * FROM source_adapters WHERE enabled ORDER BY priority, key;

-- name: GetSourceAdapter :one
SELECT * FROM source_adapters WHERE id = $1;

-- name: CreateSourceAdapter :one
INSERT INTO source_adapters (key, name, kind, domain_patterns, enabled, priority, selectors, description)
VALUES ($1, $2, 'selector', $3, $4, $5, $6, $7) RETURNING *;

-- name: UpdateSourceAdapter :one
UPDATE source_adapters SET name = $2, domain_patterns = CASE WHEN kind = 'builtin' THEN domain_patterns ELSE $3 END,
  enabled = $4, priority = $5, selectors = CASE WHEN kind = 'builtin' THEN selectors ELSE $6 END,
  description = $7, version = version + 1
WHERE id = $1 RETURNING *;

-- name: DeleteSourceAdapter :execrows
DELETE FROM source_adapters WHERE id = $1 AND kind = 'selector';

-- name: ListImportsForProduct :many
SELECT * FROM product_imports WHERE product_id = $1 OR duplicate_of_id = $1 ORDER BY created_at DESC LIMIT 50;
