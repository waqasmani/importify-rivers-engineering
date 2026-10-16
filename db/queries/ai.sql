-- name: ListAIProviders :many
SELECT * FROM ai_provider_configs ORDER BY is_default DESC, name;

-- name: GetAIProvider :one
SELECT * FROM ai_provider_configs WHERE id = $1;

-- name: GetDefaultAIProvider :one
SELECT * FROM ai_provider_configs WHERE is_default AND enabled;

-- name: CreateAIProvider :one
INSERT INTO ai_provider_configs (name, kind, base_url, model, api_key_ciphertext, is_default, enabled, is_local,
  allow_external_data, supports_vision, temperature, max_output_tokens, timeout_seconds, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING *;

-- name: UpdateAIProvider :one
UPDATE ai_provider_configs SET name = $2, kind = $3, base_url = $4, model = $5,
  api_key_ciphertext = CASE WHEN sqlc.arg(replace_key)::boolean THEN sqlc.narg('api_key_ciphertext') ELSE api_key_ciphertext END,
  enabled = $6, is_local = $7, allow_external_data = $8, supports_vision = $9, temperature = $10,
  max_output_tokens = $11, timeout_seconds = $12
WHERE id = $1 RETURNING *;

-- name: ClearDefaultAIProvider :exec
UPDATE ai_provider_configs SET is_default = false WHERE is_default;

-- name: SetDefaultAIProvider :exec
UPDATE ai_provider_configs SET is_default = true WHERE id = $1;

-- name: DeleteAIProvider :execrows
DELETE FROM ai_provider_configs WHERE id = $1;

-- name: CountEnabledAIProviders :one
SELECT count(*) FROM ai_provider_configs WHERE enabled;

-- name: EnqueueAIJob :one
INSERT INTO ai_jobs (tool, provider_config_id, provider_kind, provider_name, model, entity_type, entity_id, input,
  evidence_refs, external_data_confirmed, requested_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: ClaimAIJob :one
UPDATE ai_jobs SET status = 'running', started_at = now(), attempts = attempts + 1, locked_by = $1, locked_at = now()
WHERE id = (SELECT id FROM ai_jobs WHERE status = 'queued' ORDER BY queued_at FOR UPDATE SKIP LOCKED LIMIT 1)
RETURNING *;

-- name: FinishAIJob :exec
UPDATE ai_jobs SET status = $2, output = $3, output_text = $4, error = $5, input_tokens = $6, output_tokens = $7,
  duration_ms = $8, grounding = $9, evidence_refs = $10, model = CASE WHEN sqlc.arg(served_model)::text = '' THEN model ELSE sqlc.arg(served_model)::text END,
  finished_at = now(), locked_by = '', locked_at = NULL
WHERE id = $1;

-- name: MarkAIJobApplied :execrows
UPDATE ai_jobs SET applied_at = now(), applied_by = $2, apply_result = $3 WHERE id = $1 AND applied_at IS NULL AND status = 'succeeded';

-- name: RetryAIJob :exec
UPDATE ai_jobs SET status = 'queued', error = $2, locked_by = '', locked_at = NULL WHERE id = $1;

-- name: RequeueStaleAIJobs :execrows
UPDATE ai_jobs SET status = CASE WHEN attempts >= max_attempts THEN 'failed' ELSE 'queued' END,
  error = CASE WHEN attempts >= max_attempts THEN 'worker stopped while processing this job' ELSE error END,
  locked_by = '', locked_at = NULL
WHERE status = 'running' AND locked_at < now() - make_interval(secs => sqlc.arg(stale_seconds)::int);

-- name: GetAIJob :one
SELECT * FROM ai_jobs WHERE id = $1;

-- name: ListAIJobs :many
SELECT id, tool, status, provider_name, provider_kind, model, entity_type, entity_id, error, attempts, input_tokens,
  output_tokens, duration_ms, requested_by, queued_at, started_at, finished_at, applied_at, count(*) OVER () AS total
FROM ai_jobs
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
  AND (sqlc.narg('tool')::text IS NULL OR tool = sqlc.narg('tool'))
  AND (sqlc.narg('entity_type')::text IS NULL OR entity_type = sqlc.narg('entity_type'))
  AND (sqlc.narg('entity_id')::uuid IS NULL OR entity_id = sqlc.narg('entity_id'))
  AND (sqlc.narg('requested_by')::uuid IS NULL OR requested_by = sqlc.narg('requested_by'))
ORDER BY queued_at DESC, id
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CancelAIJob :execrows
UPDATE ai_jobs SET status = 'cancelled', finished_at = now() WHERE id = $1 AND status = 'queued';
