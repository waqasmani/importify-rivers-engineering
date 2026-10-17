-- name: CatalogSequenceLock :exec
-- Serializes sequencers for the rest of the transaction.
SELECT pg_advisory_xact_lock(7311101);

-- name: CatalogSequenceTryLock :one
SELECT pg_try_advisory_xact_lock(7311101) AS locked;

-- name: TakeCatalogChangeEvents :many
-- Removes and returns the committed raw events (rows of transactions still in progress are not
-- visible and stay for the next run).
DELETE FROM catalog_change_events RETURNING *;

-- name: InsertCatalogChange :one
INSERT INTO catalog_changes (product_id, kind, sku, changed_fields, sourcing_only, occurred_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id;

-- name: UpsertCatalogProductState :exec
INSERT INTO catalog_product_state (product_id, version, changed_at, published) VALUES ($1, $2, now(), $3)
ON CONFLICT (product_id) DO UPDATE SET version = EXCLUDED.version, changed_at = EXCLUDED.changed_at, published = EXCLUDED.published;

-- name: DeleteCatalogProductState :exec
DELETE FROM catalog_product_state WHERE product_id = ANY(sqlc.arg(product_ids)::uuid[]);

-- name: CatalogHead :one
-- The newest feed position (never lower than what retention removed) and the pruned boundary.
SELECT greatest(coalesce(max(c.id), 0), f.pruned_through)::bigint AS head, f.pruned_through
FROM catalog_feed_state f LEFT JOIN catalog_changes c ON true
GROUP BY f.pruned_through;

-- name: ListCatalogChanges :many
SELECT * FROM catalog_changes WHERE id > sqlc.arg(after_id) ORDER BY id LIMIT sqlc.arg(lim);

-- name: PruneCatalogChanges :one
WITH gone AS (
  DELETE FROM catalog_changes WHERE sequenced_at < sqlc.arg(before)::timestamptz RETURNING id
), mark AS (
  UPDATE catalog_feed_state SET pruned_through = greatest(pruned_through, (SELECT max(id) FROM gone))
  WHERE EXISTS (SELECT 1 FROM gone)
  RETURNING pruned_through
)
SELECT count(*) AS removed FROM gone;

-- name: ListCatalogProductIDs :many
-- Published products in catalog version order (keyset pagination on version).
SELECT p.id, s.version
FROM products p JOIN catalog_product_state s ON s.product_id = p.id
WHERE rea_catalog_published(p.status, p.review_status)
  AND s.version > sqlc.arg(after_version)
  AND (sqlc.narg('updated_since')::timestamptz IS NULL OR s.changed_at >= sqlc.narg('updated_since'))
  AND (sqlc.narg('category')::text IS NULL OR lower(p.category) = lower(sqlc.narg('category')))
  AND (sqlc.narg('availability')::text IS NULL OR p.availability = sqlc.narg('availability'))
  AND (sqlc.narg('source_domain')::text IS NULL OR p.source_domain = lower(sqlc.narg('source_domain')))
  AND (sqlc.narg('sku')::text IS NULL OR lower(p.sku) = lower(sqlc.narg('sku'))
       OR EXISTS (SELECT 1 FROM product_variants v WHERE v.product_id = p.id AND lower(v.sku) = lower(sqlc.narg('sku'))))
  AND (sqlc.narg('tag')::text IS NULL OR EXISTS (
        SELECT 1 FROM record_tags rt JOIN tags t ON t.id = rt.tag_id
        WHERE rt.entity_type = 'product' AND rt.entity_id = p.id AND t.name = sqlc.narg('tag')::citext))
ORDER BY s.version
LIMIT sqlc.arg(lim);

-- name: CatalogProductsByIDs :many
SELECT sqlc.embed(p), s.version, s.changed_at AS catalog_changed_at, su.name AS supplier_name
FROM products p
JOIN catalog_product_state s ON s.product_id = p.id
LEFT JOIN suppliers su ON su.id = p.supplier_id
WHERE p.id = ANY(sqlc.arg(ids)::uuid[]) AND rea_catalog_published(p.status, p.review_status);

-- name: CatalogProductVersion :one
SELECT s.version FROM products p JOIN catalog_product_state s ON s.product_id = p.id
WHERE p.id = $1 AND rea_catalog_published(p.status, p.review_status);

-- name: CatalogProductIDsBySKU :many
-- Published products whose SKU, or one of whose variant SKUs, matches (product SKUs first).
SELECT p.id, (lower(p.sku) = lower(sqlc.arg(sku)::text))::boolean AS product_match
FROM products p JOIN catalog_product_state s ON s.product_id = p.id
WHERE rea_catalog_published(p.status, p.review_status)
  AND (lower(p.sku) = lower(sqlc.arg(sku)::text)
       OR EXISTS (SELECT 1 FROM product_variants v WHERE v.product_id = p.id AND lower(v.sku) = lower(sqlc.arg(sku)::text)))
ORDER BY product_match DESC, s.version
LIMIT 10;

-- name: LatestCatalogProductID :one
SELECT s.product_id FROM catalog_product_state s JOIN products p ON p.id = s.product_id
WHERE rea_catalog_published(p.status, p.review_status)
ORDER BY s.version DESC LIMIT 1;

-- name: CatalogVariantsFor :many
SELECT * FROM product_variants WHERE product_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY product_id, position, title, id;

-- name: CatalogImagesFor :many
SELECT * FROM product_images WHERE product_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY product_id, is_primary DESC, position, id;
