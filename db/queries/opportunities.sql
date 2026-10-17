-- name: ListOpportunities :many
SELECT sqlc.embed(o), co.name AS company_name, s.name AS supplier_name,
  concat_ws(' ', ct.first_name, ct.last_name)::text AS contact_name,
  ps.name AS stage_name, ps.category AS stage_category, ps.color AS stage_color, u.display_name AS owner_name,
  count(*) OVER () AS total
FROM opportunities o
LEFT JOIN companies co ON co.id = o.company_id
LEFT JOIN suppliers s ON s.id = o.supplier_id
LEFT JOIN contacts ct ON ct.id = o.contact_id
LEFT JOIN pipeline_stages ps ON ps.id = o.pipeline_stage_id
LEFT JOIN users u ON u.id = o.owner_id
WHERE (sqlc.narg('q')::text IS NULL
       OR rea_search_doc(o.name, o.description, o.outcome_reason, '') @@ websearch_to_tsquery('simple', sqlc.narg('q'))
       OR o.name ILIKE '%' || sqlc.narg('q') || '%')
  AND (sqlc.narg('stage_id')::uuid IS NULL OR o.pipeline_stage_id = sqlc.narg('stage_id'))
  AND (sqlc.narg('pipeline_id')::uuid IS NULL OR ps.pipeline_id = sqlc.narg('pipeline_id'))
  AND (sqlc.narg('status')::text IS NULL OR o.status = sqlc.narg('status'))
  AND (sqlc.narg('owner_id')::uuid IS NULL OR o.owner_id = sqlc.narg('owner_id'))
  AND (sqlc.narg('company_id')::uuid IS NULL OR o.company_id = sqlc.narg('company_id'))
  AND (sqlc.narg('supplier_id')::uuid IS NULL OR o.supplier_id = sqlc.narg('supplier_id'))
  AND (sqlc.narg('tag_id')::uuid IS NULL OR EXISTS (
        SELECT 1 FROM record_tags rt WHERE rt.entity_type = 'opportunity' AND rt.entity_id = o.id AND rt.tag_id = sqlc.narg('tag_id')))
ORDER BY
  CASE WHEN sqlc.arg(sort)::text = 'name' THEN o.name END ASC,
  CASE WHEN sqlc.arg(sort)::text = '-amount' THEN o.amount END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort)::text = 'close' THEN o.expected_close_date END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort)::text = '-created' THEN o.created_at END DESC,
  o.updated_at DESC, o.id
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: GetOpportunity :one
SELECT * FROM opportunities WHERE id = $1;

-- name: CreateOpportunity :one
INSERT INTO opportunities (name, company_id, supplier_id, contact_id, lead_id, pipeline_stage_id, amount, currency,
                           expected_close_date, probability, status, closed_at, outcome_reason, description, owner_id,
                           custom_fields, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
RETURNING *;

-- name: UpdateOpportunity :one
UPDATE opportunities SET name = $2, company_id = $3, supplier_id = $4, contact_id = $5, pipeline_stage_id = $6,
  amount = $7, currency = $8, expected_close_date = $9, probability = $10, status = $11, closed_at = $12,
  outcome_reason = $13, description = $14, owner_id = $15, custom_fields = $16
WHERE id = $1 RETURNING *;

-- name: SetOpportunityStage :one
UPDATE opportunities SET pipeline_stage_id = $2, status = $3, closed_at = $4, probability = coalesce(sqlc.narg('probability'), probability)
WHERE id = $1 RETURNING *;

-- name: DeleteOpportunity :execrows
DELETE FROM opportunities WHERE id = $1;

-- name: DeleteOpportunitiesBulk :execrows
DELETE FROM opportunities WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: SetOpportunityOwnerBulk :execrows
UPDATE opportunities SET owner_id = sqlc.narg('owner_id') WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: ListOpportunityProducts :many
SELECT op.*, p.title AS product_title, p.sku AS product_sku
FROM opportunity_products op JOIN products p ON p.id = op.product_id
WHERE op.opportunity_id = $1 ORDER BY p.title;

-- name: UpsertOpportunityProduct :one
INSERT INTO opportunity_products (opportunity_id, product_id, quantity, unit_price, currency, notes)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (opportunity_id, product_id) DO UPDATE SET quantity = EXCLUDED.quantity, unit_price = EXCLUDED.unit_price,
  currency = EXCLUDED.currency, notes = EXCLUDED.notes
RETURNING *;

-- name: DeleteOpportunityProduct :execrows
DELETE FROM opportunity_products WHERE opportunity_id = $1 AND product_id = $2;

-- name: PipelineSummary :many
-- A stage belongs to one pipeline of one record type, so at most one of the counts is non-zero.
-- total_amount is the opportunity amount or the lead's estimated value; suppliers have none.
SELECT ps.id AS stage_id, ps.name AS stage_name, ps.category, ps.color, ps.position,
  ((SELECT count(*) FROM opportunities o WHERE o.pipeline_stage_id = ps.id)
   + (SELECT count(*) FROM leads l WHERE l.pipeline_stage_id = ps.id)
   + (SELECT count(*) FROM suppliers su WHERE su.pipeline_stage_id = ps.id))::bigint AS deal_count,
  (coalesce((SELECT sum(o.amount) FROM opportunities o WHERE o.pipeline_stage_id = ps.id), 0)
   + coalesce((SELECT sum(l.estimated_value) FROM leads l WHERE l.pipeline_stage_id = ps.id), 0))::numeric AS total_amount
FROM pipeline_stages ps
WHERE ps.pipeline_id = $1
ORDER BY ps.position;
