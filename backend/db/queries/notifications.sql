-- Notifications queries
-- All notification and preference queries are scoped by organization_id AND user_id.
-- Outbox queries are used only by DrainOutbox and domain-transaction trigger writes.


-- =============================================================================
-- OUTBOX QUERIES (used by domain repositories and DrainOutbox)
-- =============================================================================

-- name: CreateNotificationOutboxEntry :one
-- Written inside domain transactions. actor_id may be NULL for system events.
INSERT INTO notification_outbox (
    organization_id,
    event_type,
    actor_id,
    entity_type,
    entity_id,
    payload
)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: DrainOutboxEntries :many
-- Claims unprocessed outbox entries for a specific organization using
-- FOR UPDATE SKIP LOCKED so concurrent drain calls do not double-process.
-- Limited to 100 per call to bound transaction size.
SELECT *
FROM   notification_outbox
WHERE  organization_id = $1
  AND  processed_at    IS NULL
ORDER  BY created_at ASC
LIMIT  100
FOR    UPDATE SKIP LOCKED;

-- name: MarkOutboxEntryProcessed :exec
-- Called by DrainOutbox after all notifications for this entry have been inserted.
UPDATE notification_outbox
SET    processed_at = NOW()
WHERE  id = $1;

-- name: GetOrgMembersForNotification :many
-- Returns distinct user IDs for all non-expired role holders in an org.
-- Used by DrainOutbox to determine the fan-out recipient set.
-- Platform admins (organization_id IS NULL) are excluded: they are not
-- members of a specific org.
SELECT DISTINCT uor.user_id
FROM   user_organization_roles uor
WHERE  uor.organization_id = $1
  AND  (uor.expires_at IS NULL OR uor.expires_at > NOW());


-- =============================================================================
-- NOTIFICATION QUERIES (used by the notifications service / handler)
-- =============================================================================

-- name: CreateNotification :exec
-- Written ONLY by DrainOutbox. ON CONFLICT DO NOTHING is applied at the
-- application layer to make drain retries idempotent.
INSERT INTO notifications (
    organization_id,
    user_id,
    outbox_id,
    channel,
    event_type,
    entity_type,
    entity_id,
    payload,
    sent_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: GetNotificationByID :one
-- Fetches a single notification scoped by org and user.
-- deleted_at IS NULL: soft-deleted notifications are not visible.
SELECT *
FROM   notifications
WHERE  id              = $1
  AND  organization_id = $2
  AND  user_id         = $3
  AND  deleted_at      IS NULL
LIMIT  1;

-- name: ListNotificationsByUser :many
-- Paginated list of undeleted notifications for a user within an org,
-- newest first.
SELECT *
FROM   notifications
WHERE  organization_id = sqlc.arg(organization_id)
  AND  user_id         = sqlc.arg(user_id)
  AND  deleted_at      IS NULL
ORDER  BY created_at DESC
LIMIT  sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- name: CountNotificationsByUser :one
-- Count matching the same filters as ListNotificationsByUser.
SELECT COUNT(*)
FROM   notifications
WHERE  organization_id = $1
  AND  user_id         = $2
  AND  deleted_at      IS NULL;

-- name: MarkNotificationRead :one
-- Sets read_at to NOW() for a specific notification owned by the user.
-- Returns the updated row so the handler can return the new state.
UPDATE notifications
SET    read_at = NOW()
WHERE  id              = $1
  AND  organization_id = $2
  AND  user_id         = $3
  AND  deleted_at      IS NULL
  AND  read_at         IS NULL
RETURNING *;

-- name: MarkAllNotificationsRead :exec
-- Marks all unread, undeleted notifications as read for a user within an org.
UPDATE notifications
SET    read_at = NOW()
WHERE  organization_id = $1
  AND  user_id         = $2
  AND  deleted_at      IS NULL
  AND  read_at         IS NULL;

-- name: SoftDeleteNotification :execrows
-- Soft-deletes a notification by setting deleted_at.
-- Returns the number of rows affected (0 = not found / already deleted).
UPDATE notifications
SET    deleted_at = NOW()
WHERE  id              = $1
  AND  organization_id = $2
  AND  user_id         = $3
  AND  deleted_at      IS NULL;


-- =============================================================================
-- PREFERENCE QUERIES (used by the notifications service / handler)
-- =============================================================================

-- name: GetUserPreferences :many
-- Returns all preferences for a user within an org.
SELECT *
FROM   notification_preferences
WHERE  organization_id = $1
  AND  user_id         = $2
ORDER  BY event_type, channel;

-- name: GetUserPreference :one
-- Fetches a single preference row. Returns ErrNoRows when no preference is set
-- (application treats missing row as enabled = TRUE).
SELECT *
FROM   notification_preferences
WHERE  organization_id = $1
  AND  user_id         = $2
  AND  event_type      = $3
  AND  channel         = $4
LIMIT  1;

-- name: UpsertNotificationPreference :one
-- Creates or updates a preference (last-writer-wins).
INSERT INTO notification_preferences (
    organization_id,
    user_id,
    event_type,
    channel,
    enabled
)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (organization_id, user_id, event_type, channel)
DO UPDATE SET
    enabled    = EXCLUDED.enabled,
    updated_at = NOW()
RETURNING *;

-- name: GetNotificationPreferencesForEvent :many
-- Batch-loads all preference rows for a given (organization_id, event_type, channel).
-- Used by DrainOutbox to resolve preferences in memory rather than with per-user
-- round-trips: one query per unique (event_type, channel) pair across all pending
-- entries, replacing the O(entries × users) pattern with O(unique_pairs).
-- A user not present in the result set has no stored preference and is treated
-- as enabled = TRUE (the default opt-out model).
SELECT user_id, enabled
FROM   notification_preferences
WHERE  organization_id = $1
  AND  event_type      = $2
  AND  channel         = $3;
