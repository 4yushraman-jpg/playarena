-- =============================================================================
-- Migration  : 000025_notification_email_delivery (DOWN)
-- =============================================================================

DROP INDEX IF EXISTS idx_notifications_email_pending;

ALTER TABLE notifications
    DROP COLUMN IF EXISTS attempt_count,
    DROP COLUMN IF EXISTS last_attempted_at,
    DROP COLUMN IF EXISTS lease_expires_at,
    DROP COLUMN IF EXISTS failed_permanently;
