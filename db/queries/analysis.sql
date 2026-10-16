-- name: ListAnalysisProjects :many
SELECT sqlc.embed(p), w.name AS workspace_name,
  (SELECT count(*) FROM analysis_sources s WHERE s.project_id = p.id) AS source_count,
  (SELECT count(*) FROM analysis_findings f WHERE f.project_id = p.id) AS finding_count,
  (SELECT count(*) FROM analysis_reports r WHERE r.project_id = p.id) AS report_count,
  count(*) OVER () AS total
FROM analysis_projects p JOIN workspaces w ON w.id = p.workspace_id
WHERE (sqlc.narg('q')::text IS NULL
       OR rea_search_doc(p.name, p.description, p.objective, '') @@ websearch_to_tsquery('simple', sqlc.narg('q'))
       OR p.name ILIKE '%' || sqlc.narg('q') || '%')
  AND (sqlc.narg('workspace_id')::uuid IS NULL OR p.workspace_id = sqlc.narg('workspace_id'))
  AND (sqlc.narg('status')::text IS NULL OR p.status = sqlc.narg('status'))
ORDER BY p.updated_at DESC, p.id
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: GetAnalysisProject :one
SELECT * FROM analysis_projects WHERE id = $1;

-- name: CreateAnalysisProject :one
INSERT INTO analysis_projects (workspace_id, name, description, objective, status, analysis_types, authorization_basis,
  authorization_note, authorization_confirmed_by, authorization_confirmed_at, owner_id, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now(), $10, $11)
RETURNING *;

-- name: UpdateAnalysisProject :one
UPDATE analysis_projects SET workspace_id = $2, name = $3, description = $4, objective = $5, status = $6,
  analysis_types = $7, owner_id = $8
WHERE id = $1 RETURNING *;

-- name: TouchAnalysisProject :exec
UPDATE analysis_projects SET updated_at = now() WHERE id = $1;

-- name: DeleteAnalysisProject :execrows
DELETE FROM analysis_projects WHERE id = $1;

-- name: CreateAnalysisSource :one
INSERT INTO analysis_sources (project_id, kind, title, url, captured_page_id, attachment_id, content_text, content_sha256,
  mime_type, size_bytes, metadata, status, error, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING *;

-- name: GetAnalysisSource :one
SELECT * FROM analysis_sources WHERE id = $1;

-- name: ListAnalysisSources :many
SELECT id, project_id, kind, title, url, captured_page_id, attachment_id, content_sha256, mime_type, size_bytes,
  metadata, status, error, created_by, created_at, length(content_text) AS content_length
FROM analysis_sources WHERE project_id = $1 ORDER BY created_at DESC LIMIT 500;

-- name: UpdateAnalysisSourceContent :exec
UPDATE analysis_sources SET content_text = $2, content_sha256 = $3, metadata = $4, status = $5, error = $6 WHERE id = $1;

-- name: DeleteAnalysisSource :execrows
DELETE FROM analysis_sources WHERE id = $1 AND project_id = $2;

-- name: EnqueueAnalysisRun :one
INSERT INTO analysis_runs (project_id, source_id, analyzer, options, created_by) VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: ClaimAnalysisRun :one
UPDATE analysis_runs SET status = 'running', started_at = now(), attempts = attempts + 1, locked_by = $1, locked_at = now()
WHERE id = (SELECT id FROM analysis_runs WHERE status = 'queued' ORDER BY queued_at FOR UPDATE SKIP LOCKED LIMIT 1)
RETURNING *;

-- name: FinishAnalysisRun :exec
UPDATE analysis_runs SET status = $2, stats = $3, error = $4, finished_at = now(), locked_by = '', locked_at = NULL WHERE id = $1;

-- name: RequeueStaleAnalysisRuns :execrows
UPDATE analysis_runs SET status = CASE WHEN attempts >= 3 THEN 'failed' ELSE 'queued' END,
  error = CASE WHEN attempts >= 3 THEN 'worker stopped while processing this run' ELSE error END,
  locked_by = '', locked_at = NULL
WHERE status = 'running' AND locked_at < now() - make_interval(secs => sqlc.arg(stale_seconds)::int);

-- name: GetAnalysisRun :one
SELECT * FROM analysis_runs WHERE id = $1;

-- name: ListAnalysisRuns :many
SELECT r.*, s.title AS source_title FROM analysis_runs r LEFT JOIN analysis_sources s ON s.id = r.source_id
WHERE r.project_id = $1 ORDER BY r.queued_at DESC LIMIT 200;

-- name: CancelAnalysisRun :execrows
UPDATE analysis_runs SET status = 'cancelled', finished_at = now() WHERE id = $1 AND status = 'queued';

-- name: DeleteRunFindings :exec
DELETE FROM analysis_findings WHERE source_id = $1 AND origin = 'rule' AND run_id IS DISTINCT FROM $2;

-- name: CreateFinding :one
INSERT INTO analysis_findings (project_id, run_id, source_id, category, title, detail, evidence, evidence_locator,
  classification, confidence, assumptions, origin, data, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING *;

-- name: GetFinding :one
SELECT * FROM analysis_findings WHERE id = $1;

-- name: UpdateFinding :one
UPDATE analysis_findings SET category = $2, title = $3, detail = $4, evidence = $5, evidence_locator = $6,
  classification = $7, confidence = $8, assumptions = $9
WHERE id = $1 RETURNING *;

-- name: DeleteFinding :execrows
DELETE FROM analysis_findings WHERE id = $1 AND project_id = $2;

-- name: ListFindings :many
SELECT sqlc.embed(f), s.title AS source_title, s.kind AS source_kind, count(*) OVER () AS total
FROM analysis_findings f LEFT JOIN analysis_sources s ON s.id = f.source_id
WHERE f.project_id = sqlc.arg(project_id)
  AND (sqlc.narg('category')::text IS NULL OR f.category = sqlc.narg('category'))
  AND (sqlc.narg('classification')::text IS NULL OR f.classification = sqlc.narg('classification'))
  AND (sqlc.narg('source_id')::uuid IS NULL OR f.source_id = sqlc.narg('source_id'))
  AND (sqlc.narg('origin')::text IS NULL OR f.origin = sqlc.narg('origin'))
  AND (sqlc.narg('q')::text IS NULL
       OR rea_search_doc(f.title, f.detail, f.evidence, f.category) @@ websearch_to_tsquery('simple', sqlc.narg('q'))
       OR f.title ILIKE '%' || sqlc.narg('q') || '%')
ORDER BY f.category, f.confidence DESC, f.title, f.id
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: AllProjectFindings :many
SELECT f.*, s.title AS source_title, s.kind AS source_kind
FROM analysis_findings f LEFT JOIN analysis_sources s ON s.id = f.source_id
WHERE f.project_id = $1
ORDER BY f.category, f.confidence DESC, f.title
LIMIT 5000;

-- name: FindingsByIDs :many
SELECT * FROM analysis_findings WHERE project_id = $1 AND id = ANY(sqlc.arg(ids)::uuid[]);

-- name: FindingCategoryCounts :many
SELECT category, classification, count(*) AS finding_count FROM analysis_findings WHERE project_id = $1
GROUP BY category, classification ORDER BY category, classification;

-- name: CreateReport :one
INSERT INTO analysis_reports (project_id, kind, title, content_markdown, origin, is_ai_generated, ai_job_id, provider_name,
  model, finding_ids, source_ids, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- name: GetReport :one
SELECT * FROM analysis_reports WHERE id = $1;

-- name: ListReports :many
SELECT id, project_id, kind, title, origin, is_ai_generated, ai_job_id, provider_name, model, finding_ids, source_ids,
  created_by, created_at, length(content_markdown) AS content_length
FROM analysis_reports WHERE project_id = $1 ORDER BY created_at DESC LIMIT 200;

-- name: DeleteReport :execrows
DELETE FROM analysis_reports WHERE id = $1 AND project_id = $2;

-- name: SetAnalysisSourceStatus :exec
UPDATE analysis_sources SET status = $2, error = $3 WHERE id = $1;
