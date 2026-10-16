-- name: ListPipelines :many
SELECT * FROM pipelines
WHERE (sqlc.narg('entity_type')::text IS NULL OR entity_type = sqlc.narg('entity_type'))
ORDER BY entity_type, position, name;

-- name: GetPipeline :one
SELECT * FROM pipelines WHERE id = $1;

-- name: GetDefaultPipeline :one
SELECT * FROM pipelines WHERE entity_type = $1 AND is_default;

-- name: CreatePipeline :one
INSERT INTO pipelines (entity_type, name, is_default, position) VALUES ($1, $2, $3, $4) RETURNING *;

-- name: UpdatePipeline :one
UPDATE pipelines SET name = $2, position = $3 WHERE id = $1 RETURNING *;

-- name: ClearDefaultPipeline :exec
UPDATE pipelines SET is_default = false WHERE entity_type = $1 AND is_default;

-- name: SetDefaultPipeline :exec
UPDATE pipelines SET is_default = true WHERE id = $1;

-- name: DeletePipeline :execrows
DELETE FROM pipelines WHERE id = $1 AND NOT is_default;

-- name: ListStages :many
SELECT * FROM pipeline_stages WHERE pipeline_id = $1 ORDER BY position, name;

-- name: ListAllStages :many
SELECT ps.*, p.entity_type FROM pipeline_stages ps JOIN pipelines p ON p.id = ps.pipeline_id ORDER BY p.entity_type, p.position, ps.position;

-- name: GetStage :one
SELECT ps.*, p.entity_type FROM pipeline_stages ps JOIN pipelines p ON p.id = ps.pipeline_id WHERE ps.id = $1;

-- name: CreateStage :one
INSERT INTO pipeline_stages (pipeline_id, name, position, category, color, probability) VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: UpdateStage :one
UPDATE pipeline_stages SET name = $2, position = $3, category = $4, color = $5, probability = $6 WHERE id = $1 RETURNING *;

-- name: DeleteStage :execrows
DELETE FROM pipeline_stages WHERE id = $1;

-- name: FirstStageOfDefaultPipeline :one
SELECT ps.* FROM pipeline_stages ps JOIN pipelines p ON p.id = ps.pipeline_id
WHERE p.entity_type = $1 AND p.is_default ORDER BY ps.position LIMIT 1;
