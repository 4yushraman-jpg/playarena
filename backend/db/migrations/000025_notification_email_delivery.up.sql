-- =============================================================================
-- Migration  : 000025_notification_email_delivery (UP)
-- Description: Adds email delivery state tracking columns to the notifications
--              table and a supporting partial index for the EmailWorker.
--
-- Delivery state machine (email channel only):
--   Pending   → attempt_count = 0, sent_at IS NULL, failed_permanently = FALSE
--   Claimed   → attempt_count = 1/2/3, lease_expires_at = NOW() + 5 min
--   Succeeded → sent_at IS NOT NULL
--   Failed    → failed_permanently = TRUE (after max_attempts exhausted)
--
-- Retry schedule (encoded in the worker, not in the schema):
--   Attempt 1 fails → retry in 1 minute  (lease_expires_at = NOW() + 1 min)
--   Attempt 2 fails → retry in 5 minutes (lease_expires_at = NOW() + 5 min)
--   Attempt 3 fails → failed_permanently = TRUE (no further retries)
--
-- Depends on : 000022_notifications
-- =============================================================================

ALTER TABLE notifications
    ADD COLUMN attempt_count      INT         NOT NULL DEFAULT 0,
    ADD COLUMN last_attempted_at  TIMESTAMPTZ,
    ADD COLUMN lease_expires_at   TIMESTAMPTZ,
    ADD COLUMN failed_permanently BOOLEAN     NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN notifications.attempt_count IS
    'Number of email delivery attempts claimed by the EmailWorker. '
    'Incremented at claim time (before delivery). Capped at max_attempts (3). '
    'Applies only to channel = ''email''; unused for in_app.';

COMMENT ON COLUMN notifications.last_attempted_at IS
    'Timestamp of the most recent delivery attempt claim. '
    'Used to order the worker claim queue (oldest-first) and calculate retry windows.';

COMMENT ON COLUMN notifications.lease_expires_at IS
    'Soft lease expiry set by the EmailWorker when claiming a row. '
    'Prevents concurrent workers from double-delivering the same email. '
    'On successful delivery: superseded by sent_at. '
    'On failure: reset to NOW() + retry_delay by RecordEmailDeliveryFailure. '
    'On worker crash: expires naturally so another worker can retry.';

COMMENT ON COLUMN notifications.failed_permanently IS
    'Set to TRUE after max_attempts delivery attempts all fail. '
    'Rows with failed_permanently = TRUE are never retried automatically; '
    'they require manual intervention (e.g., admin reset or investigation).';

-- Partial index for the EmailWorker claim query.
-- Covers rows that are eligible for delivery:
--   channel = 'email'         — only email rows, not in_app
--   sent_at IS NULL           — not yet successfully delivered
--   failed_permanently = FALSE — not dead-lettered
-- Ordered by last_attempted_at NULLS FIRST so fresh (never-attempted) rows
-- are claimed before rows waiting for their retry window.
CREATE INDEX idx_notifications_email_pending
    ON  notifications (last_attempted_at ASC NULLS FIRST, created_at ASC)
    WHERE channel = 'email'
      AND sent_at IS NULL
      AND failed_permanently = FALSE;

COMMENT ON INDEX idx_notifications_email_pending IS
    'EmailWorker claim index: finds eligible email rows (not sent, not dead-lettered) '
    'in oldest-first order. The lease_expires_at < NOW() filter is applied as a heap '
    'check on the small result set produced by this index.';
