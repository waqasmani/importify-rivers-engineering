-- name: ListCompanies :many
SELECT sqlc.embed(c), u.display_name AS owner_name,
  (SELECT count(*) FROM contacts ct WHERE ct.company_id = c.id) AS contact_count,
  count(*) OVER () AS total
FROM companies c LEFT JOIN users u ON u.id = c.owner_id
WHERE (sqlc.narg('q')::text IS NULL
       OR rea_search_doc(c.name, c.domain, c.industry, c.description) @@ websearch_to_tsquery('simple', sqlc.narg('q'))
       OR c.normalized_name % lower(sqlc.narg('q')) OR c.name ILIKE '%' || sqlc.narg('q') || '%')
  AND (sqlc.narg('company_type')::text IS NULL OR c.company_type = sqlc.narg('company_type'))
  AND (sqlc.narg('status')::text IS NULL OR c.status = sqlc.narg('status'))
  AND (sqlc.narg('owner_id')::uuid IS NULL OR c.owner_id = sqlc.narg('owner_id'))
  AND (sqlc.narg('country')::text IS NULL OR c.country ILIKE sqlc.narg('country'))
  AND (sqlc.narg('tag_id')::uuid IS NULL OR EXISTS (
        SELECT 1 FROM record_tags rt WHERE rt.entity_type = 'company' AND rt.entity_id = c.id AND rt.tag_id = sqlc.narg('tag_id')))
ORDER BY
  CASE WHEN sqlc.arg(sort)::text = 'name' THEN c.name END ASC,
  CASE WHEN sqlc.arg(sort)::text = '-name' THEN c.name END DESC,
  CASE WHEN sqlc.arg(sort)::text = 'created' THEN c.created_at END ASC,
  CASE WHEN sqlc.arg(sort)::text = '-created' THEN c.created_at END DESC,
  c.updated_at DESC, c.id
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: GetCompany :one
SELECT * FROM companies WHERE id = $1;

-- name: CreateCompany :one
INSERT INTO companies (name, normalized_name, domain, website, industry, company_type, status, country, city, address,
                       phone, email, description, employee_count, owner_id, custom_fields, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
RETURNING *;

-- name: UpdateCompany :one
UPDATE companies SET name = $2, normalized_name = $3, domain = $4, website = $5, industry = $6, company_type = $7,
  status = $8, country = $9, city = $10, address = $11, phone = $12, email = $13, description = $14,
  employee_count = $15, owner_id = $16, custom_fields = $17
WHERE id = $1 RETURNING *;

-- name: DeleteCompany :execrows
DELETE FROM companies WHERE id = $1;

-- name: FindCompanyDuplicates :many
SELECT c.*, similarity(c.normalized_name, sqlc.arg(normalized_name)::text)::float8 AS score
FROM companies c
WHERE c.id <> coalesce(sqlc.narg('exclude_id')::uuid, '00000000-0000-0000-0000-000000000000'::uuid)
  AND ((sqlc.narg('domain')::text IS NOT NULL AND c.domain = sqlc.narg('domain'))
       OR c.normalized_name % sqlc.arg(normalized_name)::text)
ORDER BY (c.domain IS NOT DISTINCT FROM sqlc.narg('domain')) DESC, score DESC
LIMIT 10;

-- name: SetCompanyOwnerBulk :execrows
UPDATE companies SET owner_id = sqlc.narg('owner_id') WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: SetCompanyStatusBulk :execrows
UPDATE companies SET status = sqlc.arg(status) WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: DeleteCompaniesBulk :execrows
DELETE FROM companies WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: ReassignCompanyReferences :exec
WITH c1 AS (UPDATE contacts SET company_id = sqlc.arg(target_id) WHERE company_id = ANY(sqlc.arg(source_ids)::uuid[])),
     c2 AS (UPDATE suppliers SET company_id = sqlc.arg(target_id) WHERE company_id = ANY(sqlc.arg(source_ids)::uuid[])),
     c3 AS (UPDATE leads SET company_id = sqlc.arg(target_id) WHERE company_id = ANY(sqlc.arg(source_ids)::uuid[])),
     c4 AS (UPDATE opportunities SET company_id = sqlc.arg(target_id) WHERE company_id = ANY(sqlc.arg(source_ids)::uuid[]))
UPDATE products SET company_id = sqlc.arg(target_id) WHERE company_id = ANY(sqlc.arg(source_ids)::uuid[]);
