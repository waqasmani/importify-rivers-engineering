-- name: ListContacts :many
SELECT sqlc.embed(c), co.name AS company_name, s.name AS supplier_name, count(*) OVER () AS total
FROM contacts c
LEFT JOIN companies co ON co.id = c.company_id
LEFT JOIN suppliers s ON s.id = c.supplier_id
WHERE (sqlc.narg('q')::text IS NULL
       OR rea_search_doc(c.first_name, c.last_name, c.email::text, c.job_title) @@ websearch_to_tsquery('simple', sqlc.narg('q'))
       OR lower(c.full_name) % lower(sqlc.narg('q')) OR c.full_name ILIKE '%' || sqlc.narg('q') || '%'
       OR c.email::text ILIKE '%' || sqlc.narg('q') || '%')
  AND (sqlc.narg('company_id')::uuid IS NULL OR c.company_id = sqlc.narg('company_id'))
  AND (sqlc.narg('supplier_id')::uuid IS NULL OR c.supplier_id = sqlc.narg('supplier_id'))
  AND (sqlc.narg('status')::text IS NULL OR c.status = sqlc.narg('status'))
  AND (sqlc.narg('owner_id')::uuid IS NULL OR c.owner_id = sqlc.narg('owner_id'))
  AND (sqlc.narg('decision_maker')::boolean IS NULL OR c.is_decision_maker = sqlc.narg('decision_maker'))
  AND (sqlc.narg('tag_id')::uuid IS NULL OR EXISTS (
        SELECT 1 FROM record_tags rt WHERE rt.entity_type = 'contact' AND rt.entity_id = c.id AND rt.tag_id = sqlc.narg('tag_id')))
ORDER BY
  CASE WHEN sqlc.arg(sort)::text = 'name' THEN c.full_name END ASC,
  CASE WHEN sqlc.arg(sort)::text = '-name' THEN c.full_name END DESC,
  CASE WHEN sqlc.arg(sort)::text = 'created' THEN c.created_at END ASC,
  CASE WHEN sqlc.arg(sort)::text = '-created' THEN c.created_at END DESC,
  c.updated_at DESC, c.id
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: GetContact :one
SELECT * FROM contacts WHERE id = $1;

-- name: CreateContact :one
INSERT INTO contacts (first_name, last_name, email, phone, job_title, company_id, supplier_id, is_decision_maker,
                      linkedin_url, country, status, notes_summary, owner_id, custom_fields, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
RETURNING *;

-- name: UpdateContact :one
UPDATE contacts SET first_name = $2, last_name = $3, email = $4, phone = $5, job_title = $6, company_id = $7,
  supplier_id = $8, is_decision_maker = $9, linkedin_url = $10, country = $11, status = $12, notes_summary = $13,
  owner_id = $14, custom_fields = $15
WHERE id = $1 RETURNING *;

-- name: DeleteContact :execrows
DELETE FROM contacts WHERE id = $1;

-- name: FindContactDuplicates :many
SELECT c.*, (CASE WHEN c.email IS NOT NULL AND c.email = sqlc.narg('email')::citext THEN 1.0
                  ELSE similarity(lower(c.full_name), lower(sqlc.arg(full_name)::text)) END)::float8 AS score
FROM contacts c
WHERE c.id <> coalesce(sqlc.narg('exclude_id')::uuid, '00000000-0000-0000-0000-000000000000'::uuid)
  AND ((sqlc.narg('email')::citext IS NOT NULL AND c.email = sqlc.narg('email')::citext)
       OR (lower(c.full_name) % lower(sqlc.arg(full_name)::text)
           AND (sqlc.narg('company_id')::uuid IS NULL OR c.company_id IS NULL OR c.company_id = sqlc.narg('company_id'))))
ORDER BY score DESC
LIMIT 10;

-- name: SetContactOwnerBulk :execrows
UPDATE contacts SET owner_id = sqlc.narg('owner_id') WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: SetContactStatusBulk :execrows
UPDATE contacts SET status = sqlc.arg(status) WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: DeleteContactsBulk :execrows
DELETE FROM contacts WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: ReassignContactReferences :exec
WITH c1 AS (UPDATE suppliers SET primary_contact_id = sqlc.arg(target_id) WHERE primary_contact_id = ANY(sqlc.arg(source_ids)::uuid[])),
     c2 AS (UPDATE leads SET contact_id = sqlc.arg(target_id) WHERE contact_id = ANY(sqlc.arg(source_ids)::uuid[])),
     c3 AS (UPDATE opportunities SET contact_id = sqlc.arg(target_id) WHERE contact_id = ANY(sqlc.arg(source_ids)::uuid[]))
UPDATE products SET supplier_contact_id = sqlc.arg(target_id) WHERE supplier_contact_id = ANY(sqlc.arg(source_ids)::uuid[]);
