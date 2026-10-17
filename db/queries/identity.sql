-- name: CountUsers :one
SELECT count(*) FROM users;

-- name: CreateUser :one
INSERT INTO users (email, display_name, password_hash, role_id, is_owner)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: ListUsers :many
SELECT sqlc.embed(u), r.key AS role_key, r.name AS role_name
FROM users u JOIN roles r ON r.id = u.role_id
WHERE sqlc.narg(q)::text IS NULL
   OR u.email::text ILIKE '%' || sqlc.narg(q)::text || '%'
   OR u.display_name ILIKE '%' || sqlc.narg(q)::text || '%'
ORDER BY u.is_owner DESC, u.display_name
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountUsersMatching :one
SELECT count(*) FROM users u
WHERE sqlc.narg(q)::text IS NULL
   OR u.email::text ILIKE '%' || sqlc.narg(q)::text || '%'
   OR u.display_name ILIKE '%' || sqlc.narg(q)::text || '%';

-- name: UpdateUserProfile :one
UPDATE users SET display_name = $2, email = $3 WHERE id = $1 RETURNING *;

-- name: UpdateUserAdmin :one
UPDATE users SET display_name = $2, role_id = $3, is_active = $4, email = $5 WHERE id = $1 RETURNING *;

-- name: UnlockUser :execrows
UPDATE users SET failed_login_count = 0, locked_until = NULL WHERE id = $1;

-- name: UpdateUserPassword :exec
UPDATE users SET password_hash = $2, password_changed_at = now(), failed_login_count = 0, locked_until = NULL
WHERE id = $1;

-- name: RecordLoginFailure :one
UPDATE users SET
  failed_login_count = failed_login_count + 1,
  locked_until = CASE WHEN failed_login_count + 1 >= sqlc.arg(max_failures)::int
                      THEN now() + make_interval(mins => sqlc.arg(lock_minutes)::int) ELSE locked_until END
WHERE id = sqlc.arg(id)
RETURNING failed_login_count, locked_until;

-- name: RecordLoginSuccess :exec
UPDATE users SET failed_login_count = 0, locked_until = NULL, last_login_at = now() WHERE id = $1;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = $1 AND NOT is_owner;

-- name: TransferOwnership :exec
UPDATE users SET is_owner = (id = sqlc.arg(new_owner_id)::uuid) WHERE is_owner OR id = sqlc.arg(new_owner_id)::uuid;

-- name: ListRoles :many
SELECT r.*, coalesce(array_agg(rp.permission_code ORDER BY rp.permission_code) FILTER (WHERE rp.permission_code IS NOT NULL), '{}')::text[] AS permissions,
       (SELECT count(*) FROM users u WHERE u.role_id = r.id) AS user_count
FROM roles r LEFT JOIN role_permissions rp ON rp.role_id = r.id
GROUP BY r.id
ORDER BY r.is_system DESC, r.name;

-- name: GetRole :one
SELECT * FROM roles WHERE id = $1;

-- name: GetRoleByKey :one
SELECT * FROM roles WHERE key = $1;

-- name: CreateRole :one
INSERT INTO roles (key, name, description) VALUES ($1, $2, $3) RETURNING *;

-- name: UpdateRole :one
UPDATE roles SET name = $2, description = $3 WHERE id = $1 RETURNING *;

-- name: DeleteRole :execrows
DELETE FROM roles r WHERE r.id = $1 AND NOT r.is_system AND NOT EXISTS (SELECT 1 FROM users u WHERE u.role_id = r.id);

-- name: ClearRolePermissions :exec
DELETE FROM role_permissions WHERE role_id = $1;

-- name: AddRolePermissions :exec
INSERT INTO role_permissions (role_id, permission_code)
SELECT $1, unnest(sqlc.arg(codes)::text[]) ON CONFLICT DO NOTHING;

-- name: ListPermissions :many
SELECT * FROM permissions ORDER BY code;

-- name: PermissionsForRole :many
SELECT permission_code FROM role_permissions WHERE role_id = $1 ORDER BY permission_code;

-- name: CreateSession :one
INSERT INTO user_sessions (user_id, token_hash, csrf_token, ip_address, user_agent, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetActiveSessionByHash :one
SELECT sqlc.embed(s), sqlc.embed(u)
FROM user_sessions s JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1 AND s.revoked_at IS NULL AND s.expires_at > now();

-- name: TouchSession :exec
UPDATE user_sessions SET last_seen_at = now() WHERE id = $1;

-- name: RevokeSession :exec
UPDATE user_sessions SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL;

-- name: RevokeUserSessions :exec
UPDATE user_sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL;

-- name: RevokeUserSessionsExcept :exec
UPDATE user_sessions SET revoked_at = now() WHERE user_id = $1 AND id <> $2 AND revoked_at IS NULL;

-- name: ListUserSessions :many
SELECT * FROM user_sessions
WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > now()
ORDER BY last_seen_at DESC LIMIT 50;

-- name: DeleteExpiredSessions :execrows
DELETE FROM user_sessions WHERE expires_at < now() - interval '7 days' OR revoked_at < now() - interval '7 days';

-- name: GetSetting :one
SELECT * FROM app_settings WHERE key = $1;

-- name: ListSettings :many
SELECT * FROM app_settings ORDER BY key;

-- name: UpsertSetting :one
INSERT INTO app_settings (key, value, updated_by) VALUES ($1, $2, $3)
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = now()
RETURNING *;

-- name: CreateWorkspace :one
INSERT INTO workspaces (name, slug, description, created_by) VALUES ($1, $2, $3, $4) RETURNING *;

-- name: ListWorkspaces :many
SELECT w.*, (SELECT count(*) FROM analysis_projects p WHERE p.workspace_id = w.id) AS project_count
FROM workspaces w ORDER BY w.name;

-- name: GetWorkspace :one
SELECT * FROM workspaces WHERE id = $1;

-- name: UpdateWorkspace :one
UPDATE workspaces SET name = $2, slug = $3, description = $4 WHERE id = $1 RETURNING *;

-- name: DeleteWorkspace :execrows
DELETE FROM workspaces w WHERE w.id = $1 AND NOT EXISTS (SELECT 1 FROM analysis_projects p WHERE p.workspace_id = w.id);

-- name: InsertAuditLog :exec
INSERT INTO audit_logs (actor_user_id, actor_device_id, actor_label, action, entity_type, entity_id, summary, changes, ip_address, request_id,
  actor_integration_key_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, sqlc.narg(actor_integration_key_id));

-- name: ListAuditLogs :many
SELECT * FROM audit_logs
WHERE (sqlc.narg('entity_type')::text IS NULL OR entity_type = sqlc.narg('entity_type'))
  AND (sqlc.narg('entity_id')::uuid IS NULL OR entity_id = sqlc.narg('entity_id'))
  AND (sqlc.narg('actor_user_id')::uuid IS NULL OR actor_user_id = sqlc.narg('actor_user_id'))
  AND (sqlc.narg('action')::text IS NULL OR action LIKE sqlc.narg('action') || '%')
  AND (sqlc.narg('before_id')::bigint IS NULL OR id < sqlc.narg('before_id'))
ORDER BY id DESC
LIMIT sqlc.arg('lim');

-- name: CreatePairingCode :one
INSERT INTO extension_pairing_codes (user_id, code_hash, expires_at) VALUES ($1, $2, $3) RETURNING *;

-- name: ConsumePairingCode :one
UPDATE extension_pairing_codes SET used_at = now()
WHERE code_hash = $1 AND used_at IS NULL AND expires_at > now()
RETURNING *;

-- name: CreateExtensionDevice :one
INSERT INTO extension_devices (user_id, name, browser, extension_version, last_seen_at, last_ip)
VALUES ($1, $2, $3, $4, now(), $5)
RETURNING *;

-- name: GetExtensionDevice :one
SELECT * FROM extension_devices WHERE id = $1;

-- name: ListExtensionDevices :many
SELECT d.*, u.display_name AS user_name, u.email AS user_email
FROM extension_devices d JOIN users u ON u.id = d.user_id
WHERE (sqlc.narg('user_id')::uuid IS NULL OR d.user_id = sqlc.narg('user_id'))
ORDER BY d.revoked_at NULLS FIRST, d.last_seen_at DESC NULLS LAST
LIMIT 200;

-- name: TouchExtensionDevice :exec
UPDATE extension_devices SET last_seen_at = now(), last_ip = $2,
  extension_version = CASE WHEN sqlc.arg(version)::text = '' THEN extension_version ELSE sqlc.arg(version)::text END
WHERE id = $1;

-- name: RevokeExtensionDevice :execrows
UPDATE extension_devices SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL;

-- name: CreateAPICredential :one
INSERT INTO api_credentials (user_id, device_id, name, kind, token_hash, token_prefix, scopes, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetCredentialByHash :one
SELECT sqlc.embed(c), sqlc.embed(u)
FROM api_credentials c JOIN users u ON u.id = c.user_id
LEFT JOIN extension_devices d ON d.id = c.device_id
WHERE c.token_hash = $1 AND c.revoked_at IS NULL AND c.expires_at > now()
  AND (c.device_id IS NULL OR d.revoked_at IS NULL);

-- name: TouchCredential :exec
UPDATE api_credentials SET last_used_at = now() WHERE id = $1;

-- name: RevokeCredential :execrows
UPDATE api_credentials SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL;

-- name: RevokeDeviceCredentials :exec
UPDATE api_credentials SET revoked_at = now() WHERE device_id = $1 AND revoked_at IS NULL;

-- name: ListPersonalTokens :many
SELECT * FROM api_credentials WHERE user_id = $1 AND kind = 'personal' AND revoked_at IS NULL ORDER BY created_at DESC;

-- name: DeleteExpiredCredentials :execrows
DELETE FROM api_credentials WHERE expires_at < now() - interval '7 days';

-- name: GetCredentialByHashAny :one
SELECT * FROM api_credentials WHERE token_hash = $1;

-- name: ListDeviceCredentials :many
SELECT * FROM api_credentials WHERE device_id = $1 ORDER BY created_at DESC LIMIT 100;
