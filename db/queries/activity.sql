-- name: ListTasks :many
SELECT sqlc.embed(t), u.display_name AS assignee_name, count(*) OVER () AS total
FROM tasks t LEFT JOIN users u ON u.id = t.assignee_id
WHERE (sqlc.narg('q')::text IS NULL
       OR rea_search_doc(t.title, t.description, '', '') @@ websearch_to_tsquery('simple', sqlc.narg('q'))
       OR t.title ILIKE '%' || sqlc.narg('q') || '%')
  AND (sqlc.narg('status')::text IS NULL OR t.status = sqlc.narg('status'))
  AND (sqlc.narg('open_only')::boolean IS NULL OR NOT sqlc.narg('open_only') OR t.status IN ('open', 'in_progress'))
  AND (sqlc.narg('assignee_id')::uuid IS NULL OR t.assignee_id = sqlc.narg('assignee_id'))
  AND (sqlc.narg('entity_type')::text IS NULL OR t.entity_type = sqlc.narg('entity_type'))
  AND (sqlc.narg('entity_id')::uuid IS NULL OR t.entity_id = sqlc.narg('entity_id'))
  AND (sqlc.narg('due_before')::timestamptz IS NULL OR t.due_at < sqlc.narg('due_before'))
  AND (sqlc.narg('task_type')::text IS NULL OR t.task_type = sqlc.narg('task_type'))
ORDER BY
  CASE WHEN sqlc.arg(sort)::text = 'due' THEN t.due_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort)::text = '-created' THEN t.created_at END DESC,
  t.due_at ASC NULLS LAST, t.created_at DESC, t.id
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: GetTask :one
SELECT * FROM tasks WHERE id = $1;

-- name: CreateTask :one
INSERT INTO tasks (title, description, status, priority, task_type, due_at, remind_at, completed_at, assignee_id,
                   entity_type, entity_id, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- name: UpdateTask :one
UPDATE tasks SET title = $2, description = $3, status = $4, priority = $5, task_type = $6, due_at = $7, remind_at = $8,
  completed_at = $9, assignee_id = $10, entity_type = $11, entity_id = $12
WHERE id = $1 RETURNING *;

-- name: DeleteTask :execrows
DELETE FROM tasks WHERE id = $1;

-- name: SetTaskStatusBulk :execrows
UPDATE tasks SET status = sqlc.arg(status),
  completed_at = CASE WHEN sqlc.arg(status) = 'done' THEN coalesce(completed_at, now()) ELSE NULL END
WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: DeleteTasksBulk :execrows
DELETE FROM tasks WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: DueReminders :many
SELECT t.*, u.display_name AS assignee_name FROM tasks t LEFT JOIN users u ON u.id = t.assignee_id
WHERE t.status IN ('open', 'in_progress') AND t.remind_at IS NOT NULL AND t.remind_at <= now()
  AND (sqlc.narg('assignee_id')::uuid IS NULL OR t.assignee_id = sqlc.narg('assignee_id'))
ORDER BY t.remind_at LIMIT 50;

-- name: ReassignTaskEntity :exec
UPDATE tasks SET entity_id = sqlc.arg(target_id) WHERE entity_type = sqlc.arg(entity_type) AND entity_id = ANY(sqlc.arg(source_ids)::uuid[]);

-- name: ListNotes :many
SELECT n.*, u.display_name AS author_name
FROM notes n LEFT JOIN users u ON u.id = n.author_id
WHERE n.entity_type = $1 AND n.entity_id = $2
ORDER BY n.is_pinned DESC, n.created_at DESC
LIMIT 500;

-- name: GetNote :one
SELECT * FROM notes WHERE id = $1;

-- name: CreateNote :one
INSERT INTO notes (entity_type, entity_id, parent_id, body, is_pinned, author_id) VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: UpdateNote :one
UPDATE notes SET body = $2, is_pinned = $3 WHERE id = $1 RETURNING *;

-- name: DeleteNote :execrows
DELETE FROM notes WHERE id = $1;

-- name: ReassignNotes :exec
UPDATE notes SET entity_id = sqlc.arg(target_id) WHERE entity_type = sqlc.arg(entity_type) AND entity_id = ANY(sqlc.arg(source_ids)::uuid[]);

-- name: CreateAttachment :one
INSERT INTO attachments (entity_type, entity_id, filename, content_type, size_bytes, sha256, storage_key, uploaded_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;

-- name: GetAttachment :one
SELECT * FROM attachments WHERE id = $1;

-- name: ListAttachments :many
SELECT a.*, u.display_name AS uploaded_by_name FROM attachments a LEFT JOIN users u ON u.id = a.uploaded_by
WHERE a.entity_type = $1 AND a.entity_id = $2 ORDER BY a.created_at DESC LIMIT 500;

-- name: DeleteAttachment :execrows
DELETE FROM attachments WHERE id = $1;

-- name: ReassignAttachments :exec
UPDATE attachments SET entity_id = sqlc.arg(target_id) WHERE entity_type = sqlc.arg(entity_type) AND entity_id = ANY(sqlc.arg(source_ids)::uuid[]);

-- name: StorageKeyInUse :one
SELECT EXISTS (SELECT 1 FROM attachments WHERE storage_key = $1);

-- name: ListAllStorageKeys :many
SELECT DISTINCT storage_key FROM attachments;

-- name: ListTags :many
SELECT t.*, (SELECT count(*) FROM record_tags rt WHERE rt.tag_id = t.id) AS usage_count FROM tags t ORDER BY t.name;

-- name: GetTagByName :one
SELECT * FROM tags WHERE name = $1;

-- name: CreateTag :one
INSERT INTO tags (name, color) VALUES ($1, $2) RETURNING *;

-- name: UpsertTag :one
INSERT INTO tags (name, color) VALUES ($1, $2) ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name RETURNING *;

-- name: UpdateTag :one
UPDATE tags SET name = $2, color = $3 WHERE id = $1 RETURNING *;

-- name: DeleteTag :execrows
DELETE FROM tags WHERE id = $1;

-- name: TagsForRecord :many
SELECT t.* FROM tags t JOIN record_tags rt ON rt.tag_id = t.id WHERE rt.entity_type = $1 AND rt.entity_id = $2 ORDER BY t.name;

-- name: TagsForRecords :many
SELECT rt.entity_id, t.id, t.name, t.color FROM record_tags rt JOIN tags t ON t.id = rt.tag_id
WHERE rt.entity_type = sqlc.arg(entity_type) AND rt.entity_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY t.name;

-- name: AddRecordTags :exec
INSERT INTO record_tags (tag_id, entity_type, entity_id)
SELECT unnest(sqlc.arg(tag_ids)::uuid[]), sqlc.arg(entity_type), unnest(sqlc.arg(entity_ids)::uuid[])
ON CONFLICT DO NOTHING;

-- name: TagRecordsBulk :exec
INSERT INTO record_tags (tag_id, entity_type, entity_id)
SELECT sqlc.arg(tag_id)::uuid, sqlc.arg(entity_type)::text, e FROM unnest(sqlc.arg(entity_ids)::uuid[]) AS e
ON CONFLICT DO NOTHING;

-- name: UntagRecordsBulk :exec
DELETE FROM record_tags WHERE tag_id = sqlc.arg(tag_id) AND entity_type = sqlc.arg(entity_type) AND entity_id = ANY(sqlc.arg(entity_ids)::uuid[]);

-- name: ClearRecordTags :exec
DELETE FROM record_tags WHERE entity_type = $1 AND entity_id = $2;

-- name: ReassignRecordTags :exec
INSERT INTO record_tags (tag_id, entity_type, entity_id)
SELECT DISTINCT tag_id, entity_type, sqlc.arg(target_id)::uuid FROM record_tags
WHERE record_tags.entity_type = sqlc.arg(entity_type) AND record_tags.entity_id = ANY(sqlc.arg(source_ids)::uuid[])
ON CONFLICT DO NOTHING;

-- name: ListCustomFields :many
SELECT * FROM custom_field_definitions
WHERE (sqlc.narg('entity_type')::text IS NULL OR entity_type = sqlc.narg('entity_type'))
ORDER BY entity_type, position, label;

-- name: CreateCustomField :one
INSERT INTO custom_field_definitions (entity_type, key, label, field_type, options, required, position)
VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING *;

-- name: UpdateCustomField :one
UPDATE custom_field_definitions SET label = $2, field_type = $3, options = $4, required = $5, position = $6 WHERE id = $1 RETURNING *;

-- name: DeleteCustomField :execrows
DELETE FROM custom_field_definitions WHERE id = $1;

-- name: ListSavedFilters :many
SELECT * FROM saved_filters WHERE entity_type = $1 AND (user_id = $2 OR is_shared) ORDER BY name;

-- name: CreateSavedFilter :one
INSERT INTO saved_filters (user_id, entity_type, name, filters, is_shared) VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: DeleteSavedFilter :execrows
DELETE FROM saved_filters WHERE id = $1 AND user_id = $2;

-- name: LinkAttachment :exec
UPDATE attachments SET entity_type = $2, entity_id = $3 WHERE id = $1;
