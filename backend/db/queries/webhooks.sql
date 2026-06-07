-- Webhook queries
-- All endpoint queries are scoped by organization_id.
-- Delivery queries are used only by WebhookWorker and DrainOutbox.


-- =============================================================================
-- ENDPOINT QUERIES (used by internal/webhooks)
-- =============================================================================

-- name: CreateWebhookEndpoint :one
INSERT INTO webhook_endpoints (
    organization_id,
    url,
    secret_ciphertext,
    description,
    active,
    created_by
)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetWebhookEndpointByID :one
SELECT *
FROM   webhook_endpoints
WHERE  id              = $1
  AND  organization_id = $2
LIMIT  1;

-- name: ListWebhookEndpoints :many
SELECT *
FROM   webhook_endpoints
WHERE  organization_id = $1
ORDER  BY created_at DESC;

-- name: UpdateWebhookEndpointActive :one
-- Toggle active/inactive without deleting. Returns updated row.
UPDATE webhook_endpoints
SET    active     = $3,
       updated_at = NOW()
WHERE  id              = $1
  AND  organization_id = $2
RETURNING *;

-- name: DeleteWebhookEndpoint :execrows
DELETE FROM webhook_endpoints
WHERE  id              = $1
  AND  organization_id = $2;

-- name: GetActiveWebhookEndpointsForOrg :many
-- Returns all active endpoints for an organization.
-- Called by DrainOutbox to determine the webhook fan-out set.
SELECT *
FROM   webhook_endpoints
WHERE  organization_id = $1
  AND  active          = TRUE;


-- =============================================================================
-- DELIVERY QUERIES (used by internal/webhookworker and DrainOutbox)
-- =============================================================================

-- name: CreateWebhookDelivery :one
-- Written by DrainOutbox for each active webhook endpoint in the org.
-- ON CONFLICT DO NOTHING makes retries of DrainOutbox idempotent.
INSERT INTO webhook_deliveries (
    organization_id,
    endpoint_id,
    outbox_id,
    event_type,
    entity_type,
    entity_id,
    payload
)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (outbox_id, endpoint_id) DO NOTHING
RETURNING *;

-- name: ClaimWebhookDeliveriesForDelivery :many
-- Claims up to batch_size pending webhook deliveries using FOR UPDATE SKIP LOCKED.
-- Increments attempt_count and sets a 5-minute soft lease.
-- Only rows with attempt_count < max_attempts are eligible.
UPDATE webhook_deliveries wd
SET last_attempted_at = NOW(),
    lease_expires_at  = NOW() + INTERVAL '5 minutes',
    attempt_count     = wd.attempt_count + 1
WHERE wd.id IN (
    SELECT q.id
    FROM   webhook_deliveries q
    WHERE  q.sent_at            IS NULL
      AND  q.failed_permanently = FALSE
      AND  q.attempt_count      < sqlc.arg(max_attempts)
      AND  (q.lease_expires_at  IS NULL OR q.lease_expires_at < NOW())
    ORDER  BY q.last_attempted_at ASC NULLS FIRST, q.created_at ASC
    LIMIT  sqlc.arg(batch_size)
    FOR    UPDATE SKIP LOCKED
)
RETURNING *;

-- name: RecordWebhookDeliverySuccess :exec
-- Marks a webhook delivery as successfully delivered.
-- sent_at IS NULL guard: idempotent against double-recording on worker crash.
UPDATE webhook_deliveries
SET sent_at = NOW()
WHERE id      = $1
  AND sent_at IS NULL;

-- name: RecordWebhookDeliveryFailure :exec
-- Records a failed delivery. failed_permanently = TRUE when max_attempts exhausted.
-- lease_expires_at is advanced to next_attempt_at so the claim query skips
-- this row until the retry window elapses.
UPDATE webhook_deliveries
SET failed_permanently = sqlc.arg(failed_permanently),
    lease_expires_at   = sqlc.arg(next_attempt_at)
WHERE id = sqlc.arg(id);

-- name: GetWebhookEndpointForDelivery :one
-- Fetches the endpoint referenced by a delivery row (worker needs URL + secret).
SELECT e.*
FROM   webhook_endpoints e
       JOIN webhook_deliveries d ON d.endpoint_id = e.id
WHERE  d.id = $1
LIMIT  1;

-- name: DeleteOldWebhookDeliveries :exec
-- Deletes delivered webhook_deliveries rows older than the retention cutoff.
-- Called by the cleanup scheduler to prevent unbounded table growth.
DELETE FROM webhook_deliveries
WHERE sent_at IS NOT NULL
  AND sent_at < sqlc.arg(older_than);
