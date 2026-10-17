-- ---- integration keys ----

-- name: CreateIntegrationKey :one
INSERT INTO integration_keys (name, description, token_hash, token_prefix, scopes, expires_at, created_by, created_by_label)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetActiveIntegrationKeyByHash :one
SELECT * FROM integration_keys
WHERE token_hash = $1 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now());

-- name: GetIntegrationKey :one
SELECT * FROM integration_keys WHERE id = $1;

-- name: ListIntegrationKeys :many
SELECT k.*, u.display_name AS created_by_name,
  coalesce((SELECT sum(x.requests) FROM integration_key_usage x WHERE x.key_id = k.id AND x.day = current_date), 0)::bigint AS requests_today,
  coalesce((SELECT sum(x.requests) FROM integration_key_usage x WHERE x.key_id = k.id AND x.day > current_date - 30), 0)::bigint AS requests_30d
FROM integration_keys k LEFT JOIN users u ON u.id = k.created_by
ORDER BY k.revoked_at IS NOT NULL, k.created_at DESC
LIMIT 500;

-- name: TouchIntegrationKey :one
-- Records the last use and reports whether this is the first use in the current hour or from a
-- new address, which is when an audit event is written.
WITH prev AS (
  SELECT k.id, k.last_used_at, k.last_used_ip FROM integration_keys k WHERE k.id = sqlc.arg(id) FOR UPDATE
)
UPDATE integration_keys k SET last_used_at = now(), last_used_ip = sqlc.arg(ip)
FROM prev WHERE k.id = prev.id
RETURNING (prev.last_used_at IS NULL OR prev.last_used_at < date_trunc('hour', now()) OR prev.last_used_ip <> sqlc.arg(ip))::boolean AS audit;

-- name: CountIntegrationKeyRequest :exec
INSERT INTO integration_key_usage (key_id, day, requests) VALUES ($1, current_date, 1)
ON CONFLICT (key_id, day) DO UPDATE SET requests = integration_key_usage.requests + 1;

-- name: RevokeIntegrationKey :one
UPDATE integration_keys SET revoked_at = now(), revoked_by = $2 WHERE id = $1 AND revoked_at IS NULL RETURNING *;

-- name: PruneIntegrationKeyUsage :execrows
DELETE FROM integration_key_usage WHERE day < sqlc.arg(before)::date;

-- ---- webhook endpoints ----

-- name: CreateWebhookEndpoint :one
INSERT INTO webhook_endpoints (name, url, events, enabled, include_costs, secret_ciphertext, secret_prefix, change_cursor, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7,
  greatest((SELECT coalesce(max(id), 0) FROM catalog_changes), (SELECT pruned_through FROM catalog_feed_state)), $8)
RETURNING *;

-- name: ListWebhookEndpoints :many
SELECT e.*,
  (SELECT count(*) FROM webhook_deliveries d WHERE d.endpoint_id = e.id AND d.status IN ('pending', 'delivering'))::bigint AS pending_deliveries
FROM webhook_endpoints e ORDER BY e.created_at;

-- name: CountWebhookEndpoints :one
SELECT count(*) FROM webhook_endpoints;

-- name: CountPendingWebhookDeliveries :one
SELECT count(*) FROM webhook_deliveries WHERE endpoint_id = $1 AND status IN ('pending', 'delivering');

-- name: GetWebhookEndpoint :one
SELECT * FROM webhook_endpoints WHERE id = $1;

-- name: UpdateWebhookEndpoint :one
-- Re-enabling clears the automatic-disable state; the endpoint resumes at its change cursor.
UPDATE webhook_endpoints SET name = $2, url = $3, events = $4, include_costs = $5, enabled = sqlc.arg(enabled),
  disabled_reason = CASE WHEN sqlc.arg(enabled) THEN '' ELSE disabled_reason END,
  disabled_at = CASE WHEN sqlc.arg(enabled) THEN NULL WHEN enabled THEN now() ELSE disabled_at END,
  consecutive_failures = CASE WHEN sqlc.arg(enabled) AND NOT enabled THEN 0 ELSE consecutive_failures END,
  failing_since = CASE WHEN sqlc.arg(enabled) AND NOT enabled THEN NULL ELSE failing_since END,
  updated_at = now()
WHERE id = $1 RETURNING *;

-- name: RotateWebhookSecret :one
UPDATE webhook_endpoints SET previous_secret_ciphertext = secret_ciphertext,
  previous_secret_expires_at = sqlc.arg(previous_expires_at), secret_ciphertext = sqlc.arg(secret_ciphertext),
  secret_prefix = sqlc.arg(secret_prefix), updated_at = now()
WHERE id = sqlc.arg(id) RETURNING *;

-- name: DeleteWebhookEndpoint :execrows
DELETE FROM webhook_endpoints WHERE id = $1;

-- name: ListFanOutEndpointIDs :many
SELECT e.id FROM webhook_endpoints e
WHERE e.enabled AND e.change_cursor < (SELECT coalesce(max(c.id), 0) FROM catalog_changes c)
ORDER BY e.change_cursor;

-- name: LockWebhookEndpointForFanOut :one
SELECT * FROM webhook_endpoints WHERE id = $1 AND enabled FOR UPDATE SKIP LOCKED;

-- name: AdvanceWebhookCursor :exec
UPDATE webhook_endpoints SET change_cursor = sqlc.arg(change_cursor) WHERE id = sqlc.arg(id) AND change_cursor < sqlc.arg(change_cursor);

-- name: RecordWebhookSuccess :exec
UPDATE webhook_endpoints SET consecutive_failures = 0, failing_since = NULL, last_success_at = now() WHERE id = $1;

-- name: RecordWebhookFailure :one
UPDATE webhook_endpoints SET consecutive_failures = consecutive_failures + 1, failing_since = coalesce(failing_since, now()),
  last_failure_at = now()
WHERE id = $1 RETURNING *;

-- name: DisableWebhookEndpoint :execrows
UPDATE webhook_endpoints SET enabled = false, disabled_reason = $2, disabled_at = now(), updated_at = now()
WHERE id = $1 AND enabled;

-- ---- webhook deliveries ----

-- name: CreateWebhookDelivery :one
INSERT INTO webhook_deliveries (endpoint_id, event_id, event_type, change_id, product_id, payload, is_test, max_attempts,
  redelivery_of, status, attempts, locked_by, locked_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, sqlc.arg(status), sqlc.arg(attempts), sqlc.arg(locked_by),
  CASE WHEN sqlc.arg(status)::text = 'delivering' THEN now() END)
RETURNING *;

-- name: ClaimWebhookDelivery :one
UPDATE webhook_deliveries SET status = 'delivering', attempts = attempts + 1, locked_by = $1, locked_at = now()
WHERE id = (
  SELECT d.id FROM webhook_deliveries d JOIN webhook_endpoints e ON e.id = d.endpoint_id
  WHERE d.status = 'pending' AND d.next_attempt_at <= now() AND e.enabled
  ORDER BY d.next_attempt_at, d.created_at
  FOR UPDATE OF d SKIP LOCKED LIMIT 1)
RETURNING *;

-- name: FinishWebhookDelivery :one
UPDATE webhook_deliveries SET status = sqlc.arg(status), next_attempt_at = sqlc.arg(next_attempt_at),
  last_attempt_at = now(), response_status = sqlc.narg(response_status), response_body = sqlc.arg(response_body),
  latency_ms = sqlc.arg(latency_ms), error = sqlc.arg(error), locked_by = '', locked_at = NULL,
  finished_at = CASE WHEN sqlc.arg(status)::text IN ('succeeded', 'failed') THEN now() END
WHERE id = sqlc.arg(id) RETURNING *;

-- name: InsertWebhookAttempt :exec
INSERT INTO webhook_delivery_attempts (delivery_id, attempt, status_code, latency_ms, error, response_body)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetWebhookDelivery :one
SELECT * FROM webhook_deliveries WHERE id = $1;

-- name: ListWebhookDeliveries :many
SELECT id, endpoint_id, event_id, event_type, change_id, product_id, is_test, status, attempts, max_attempts, next_attempt_at,
  last_attempt_at, response_status, latency_ms, error, redelivery_of, created_at, finished_at,
  count(*) OVER () AS total
FROM webhook_deliveries
WHERE endpoint_id = sqlc.arg(endpoint_id) AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
ORDER BY created_at DESC, id
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: ListWebhookAttempts :many
SELECT * FROM webhook_delivery_attempts WHERE delivery_id = $1 ORDER BY attempt, id;

-- name: RequeueStaleWebhookDeliveries :execrows
UPDATE webhook_deliveries SET status = 'pending', locked_by = '', locked_at = NULL, next_attempt_at = now()
WHERE status = 'delivering' AND locked_at < now() - make_interval(secs => sqlc.arg(stale_seconds)::int);

-- name: PruneWebhookDeliveries :execrows
DELETE FROM webhook_deliveries WHERE finished_at IS NOT NULL AND finished_at < sqlc.arg(before)::timestamptz;
