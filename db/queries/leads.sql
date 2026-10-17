-- name: ListLeads :many
SELECT sqlc.embed(l), co.name AS company_name, concat_ws(' ', ct.first_name, ct.last_name)::text AS contact_name,
  ps.name AS stage_name, ps.category AS stage_category, ps.color AS stage_color, u.display_name AS owner_name,
  count(*) OVER () AS total
FROM leads l
LEFT JOIN companies co ON co.id = l.company_id
LEFT JOIN contacts ct ON ct.id = l.contact_id
LEFT JOIN pipeline_stages ps ON ps.id = l.pipeline_stage_id
LEFT JOIN users u ON u.id = l.owner_id
WHERE (sqlc.narg('q')::text IS NULL
       OR rea_search_doc(l.title, l.source, l.description, '') @@ websearch_to_tsquery('simple', sqlc.narg('q'))
       OR l.title ILIKE '%' || sqlc.narg('q') || '%')
  AND (sqlc.narg('stage_id')::uuid IS NULL OR l.pipeline_stage_id = sqlc.narg('stage_id'))
  AND (sqlc.narg('pipeline_id')::uuid IS NULL OR ps.pipeline_id = sqlc.narg('pipeline_id'))
  AND (sqlc.narg('source')::text IS NULL OR l.source = sqlc.narg('source'))
  AND (sqlc.narg('owner_id')::uuid IS NULL OR l.owner_id = sqlc.narg('owner_id'))
  AND (sqlc.narg('company_id')::uuid IS NULL OR l.company_id = sqlc.narg('company_id'))
  AND (sqlc.narg('tag_id')::uuid IS NULL OR EXISTS (
        SELECT 1 FROM record_tags rt WHERE rt.entity_type = 'lead' AND rt.entity_id = l.id AND rt.tag_id = sqlc.narg('tag_id')))
ORDER BY
  CASE WHEN sqlc.arg(sort)::text = 'title' THEN l.title END ASC,
  CASE WHEN sqlc.arg(sort)::text = '-value' THEN l.estimated_value END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort)::text = '-created' THEN l.created_at END DESC,
  l.updated_at DESC, l.id
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: GetLead :one
SELECT * FROM leads WHERE id = $1;

-- name: CreateLead :one
INSERT INTO leads (title, company_id, contact_id, source, pipeline_stage_id, estimated_value, currency, description,
                   owner_id, custom_fields, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: UpdateLead :one
UPDATE leads SET title = $2, company_id = $3, contact_id = $4, source = $5, pipeline_stage_id = $6, estimated_value = $7,
  currency = $8, description = $9, owner_id = $10, custom_fields = $11
WHERE id = $1 RETURNING *;

-- name: SetLeadStage :one
UPDATE leads SET pipeline_stage_id = $2 WHERE id = $1 RETURNING *;

-- name: MarkLeadConverted :one
UPDATE leads SET converted_at = now(), converted_opportunity_id = $2, pipeline_stage_id = coalesce(sqlc.narg('stage_id'), pipeline_stage_id)
WHERE id = $1 RETURNING *;

-- name: DeleteLead :execrows
DELETE FROM leads WHERE id = $1;

-- name: DeleteLeadsBulk :execrows
DELETE FROM leads WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: SetLeadStageBulk :execrows
UPDATE leads SET pipeline_stage_id = sqlc.narg('stage_id') WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: SetLeadOwnerBulk :execrows
UPDATE leads SET owner_id = sqlc.narg('owner_id') WHERE id = ANY(sqlc.arg(ids)::uuid[]);
