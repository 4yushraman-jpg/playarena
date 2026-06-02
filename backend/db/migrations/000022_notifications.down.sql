-- =============================================================================
-- Migration  : 000022_notifications (DOWN)
-- Description: Reverses the notification system introduced in the UP migration.
--              Removes all notification tables, ENUMs, and permission seeds.
-- =============================================================================

-- Remove role grants for notification.manage.
DELETE FROM role_permissions
WHERE permission_id = (SELECT id FROM permissions WHERE slug = 'notification.manage');

-- Remove the notification.manage permission.
DELETE FROM permissions WHERE slug = 'notification.manage';

-- Drop tables (CASCADE removes indexes and constraints automatically).
DROP TABLE IF EXISTS notification_preferences CASCADE;
DROP TABLE IF EXISTS notifications CASCADE;
DROP TABLE IF EXISTS notification_outbox CASCADE;

-- Drop ENUM types (must be last; tables referencing them are gone by now).
DROP TYPE IF EXISTS notification_channel;
DROP TYPE IF EXISTS notification_event_type;
