-- =============================================================================
-- Migration  : 000022_notifications (UP)
-- Description: Implements the Transactional Outbox notification system for
--              PlayArena. Introduces two new ENUM types, three tables, all
--              supporting indexes, the notification.manage permission, and
--              role grants.
--
-- Architecture:
--   notification_outbox  — written inside domain transactions (atomically with
--                          the triggering domain write). Never read by clients.
--   notifications        — written ONLY by DrainOutbox after the domain
--                          transaction commits. Read by clients via the API.
--   notification_preferences — per-user, per-event-type, per-channel opt-in/out.
--
-- Idempotency:
--   UNIQUE (outbox_id, user_id, channel) on notifications ensures that a drain
--   retry can never produce duplicate rows. ON CONFLICT DO NOTHING at the
--   application layer makes the drain safe to retry.
--
-- Depends on : 000002 (organizations), 000003 (users), 000017 (seed_rbac)
-- =============================================================================


-- =============================================================================
-- SECTION 1: ENUM TYPES
-- =============================================================================

-- Domain events that can trigger notifications.
CREATE TYPE notification_event_type AS ENUM (
    'match_created',
    'match_started',
    'match_completed',
    'match_cancelled',
    'match_abandoned',
    'tournament_status_changed',
    'registration_approved',
    'registration_rejected',
    'registration_withdrawn'
);

COMMENT ON TYPE notification_event_type IS
    'Domain events that trigger notifications. '
    'match_*: lifecycle transitions on match fixtures. '
    'tournament_status_changed: any tournament status transition. '
    'registration_*: registration lifecycle changes.';


-- Delivery channel for a notification.
-- in_app: persisted in the notifications table; sent_at is set to NOW() on drain.
-- email: future async delivery; sent_at remains NULL until email confirmed sent.
-- webhook: future org-configurable HTTP callback; sent_at remains NULL until confirmed.
CREATE TYPE notification_channel AS ENUM (
    'in_app',
    'email',
    'webhook'
);

COMMENT ON TYPE notification_channel IS
    'Delivery channel for a notification. '
    'in_app: DB-persisted; read via API; sent_at = NOW() on drain. '
    'email: future transactional email; sent_at NULL until confirmed sent. '
    'webhook: future org-configured HTTP callback; sent_at NULL until confirmed.';


-- =============================================================================
-- SECTION 2: NOTIFICATION OUTBOX TABLE
-- =============================================================================
-- Written atomically inside domain transactions (matches, tournaments,
-- tournament_registrations). Never written by the notifications service itself.
-- Read exclusively by DrainOutbox using FOR UPDATE SKIP LOCKED.

CREATE TABLE notification_outbox (
    id              UUID        NOT NULL DEFAULT gen_random_uuid(),
    organization_id UUID        NOT NULL,
    event_type      notification_event_type NOT NULL,
    -- User who performed the triggering action. NULL for system-initiated events.
    -- SET NULL on user deletion: outbox entries are retained for drain completion.
    actor_id        UUID,
    -- Matches the entity_type/entity_id in the triggering domain table.
    entity_type     TEXT        NOT NULL,
    entity_id       UUID        NOT NULL,
    -- Event-specific structured data: previous status, new status, participant IDs, etc.
    payload         JSONB       NOT NULL DEFAULT '{}',
    -- Set by DrainOutbox to NOW() after the entry has been fully fanned out.
    -- NULL means the entry is pending processing.
    processed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT pk_notification_outbox PRIMARY KEY (id),
    CONSTRAINT fk_notif_outbox_org
        FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
    CONSTRAINT fk_notif_outbox_actor
        FOREIGN KEY (actor_id) REFERENCES users(id) ON DELETE SET NULL,
    CONSTRAINT chk_notif_outbox_entity_type
        CHECK (char_length(trim(entity_type)) >= 1)
);

COMMENT ON TABLE notification_outbox IS
    'Transactional outbox for domain-event notifications. '
    'Rows are written inside domain transactions (matches, tournaments, registrations). '
    'DrainOutbox reads pending rows (processed_at IS NULL) using FOR UPDATE SKIP LOCKED '
    'and fans them out into the notifications table. '
    'All rows are retained permanently for auditability.';

COMMENT ON COLUMN notification_outbox.processed_at IS
    'Set to NOW() by DrainOutbox after all notifications for this entry have been '
    'inserted. NULL = pending. Once set, the entry is never modified again.';

-- Index used by DrainOutbox: claim pending entries in creation order, org-scoped.
-- Partial index on processed_at IS NULL keeps it small as entries are processed.
CREATE INDEX idx_notif_outbox_pending
    ON notification_outbox (organization_id, created_at ASC)
    WHERE processed_at IS NULL;

COMMENT ON INDEX idx_notif_outbox_pending IS
    'Partial index for DrainOutbox: selects pending (processed_at IS NULL) entries '
    'in FIFO order within an org. Stays compact as entries are marked processed.';


-- =============================================================================
-- SECTION 3: NOTIFICATIONS TABLE
-- =============================================================================
-- Written ONLY by DrainOutbox. Never written directly by domain code.
-- Read by authenticated users via the notifications API (org + user scoped).
--
-- sent_at contract (enforced at the application layer):
--   channel = 'in_app'   → sent_at = NOW()  (set by DrainOutbox on insert)
--   channel = 'email'    → sent_at NULL      (set by future email worker on send)
--   channel = 'webhook'  → sent_at NULL      (set by future webhook worker on delivery)

CREATE TABLE notifications (
    id              UUID        NOT NULL DEFAULT gen_random_uuid(),
    organization_id UUID        NOT NULL,
    user_id         UUID        NOT NULL,
    -- FK back to the outbox entry that produced this notification.
    -- CASCADE: if an outbox entry is deleted (future cleanup), its notifications go too.
    outbox_id       UUID        NOT NULL,
    channel         notification_channel        NOT NULL,
    event_type      notification_event_type     NOT NULL,
    entity_type     TEXT        NOT NULL,
    entity_id       UUID        NOT NULL,
    -- Mirrors notification_outbox.payload; denormalized for query efficiency.
    payload         JSONB       NOT NULL DEFAULT '{}',
    -- Set by the client (PATCH .../read). NULL = unread.
    read_at         TIMESTAMPTZ,
    -- Set per the sent_at contract above. NULL = not yet delivered.
    sent_at         TIMESTAMPTZ,
    -- Soft-delete: set by DELETE endpoint. NULL = visible.
    deleted_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT pk_notifications PRIMARY KEY (id),
    CONSTRAINT fk_notif_org
        FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
    CONSTRAINT fk_notif_user
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT fk_notif_outbox
        FOREIGN KEY (outbox_id) REFERENCES notification_outbox(id) ON DELETE CASCADE,

    -- Drain idempotency: a retry of DrainOutbox for the same outbox entry cannot
    -- produce a duplicate row for the same user on the same channel.
    CONSTRAINT uq_notifications_outbox_user_channel
        UNIQUE (outbox_id, user_id, channel),

    CONSTRAINT chk_notif_entity_type
        CHECK (char_length(trim(entity_type)) >= 1)
);

COMMENT ON TABLE notifications IS
    'Personal notification inbox. Written ONLY by DrainOutbox — never by domain code directly. '
    'Every row is scoped to one user within one organization. '
    'UNIQUE (outbox_id, user_id, channel) ensures a drain retry is idempotent.';

COMMENT ON COLUMN notifications.sent_at IS
    'Delivery timestamp. Contract: in_app → set to NOW() on drain insert; '
    'email/webhook → NULL until the respective delivery worker confirms send.';

COMMENT ON COLUMN notifications.deleted_at IS
    'Soft-delete timestamp. NULL = visible. Set by DELETE endpoint. '
    'Deleted notifications are excluded from list queries but retained for audit.';

COMMENT ON CONSTRAINT uq_notifications_outbox_user_channel ON notifications IS
    'Drain idempotency guard. If DrainOutbox is retried for the same outbox entry, '
    'the second INSERT for (outbox_id, user_id, channel) is silently ignored '
    'via ON CONFLICT DO NOTHING at the application layer.';

-- Primary query index: list undeleted notifications for a user within an org,
-- newest first. This is the hot path for the GET /notifications endpoint.
CREATE INDEX idx_notifications_user_org
    ON notifications (organization_id, user_id, created_at DESC)
    WHERE deleted_at IS NULL;

-- Unread count index: fast COUNT(*) for unread badge display.
CREATE INDEX idx_notifications_unread
    ON notifications (organization_id, user_id)
    WHERE read_at IS NULL AND deleted_at IS NULL;

COMMENT ON INDEX idx_notifications_user_org IS
    'Hot-path index for GET /notifications: lists undeleted notifications for '
    'a user within an org, ordered newest-first.';

COMMENT ON INDEX idx_notifications_unread IS
    'Supports fast unread-count queries for notification badge display.';


-- =============================================================================
-- SECTION 4: NOTIFICATION PREFERENCES TABLE
-- =============================================================================
-- Per-user, per-org, per-event-type, per-channel opt-in/out.
-- Default (no row): notifications enabled.
-- A row with enabled = FALSE: user has opted out of that combination.
-- UPSERT semantics (last-writer-wins) at the application layer.

CREATE TABLE notification_preferences (
    id              UUID        NOT NULL DEFAULT gen_random_uuid(),
    organization_id UUID        NOT NULL,
    user_id         UUID        NOT NULL,
    event_type      notification_event_type NOT NULL,
    channel         notification_channel    NOT NULL,
    enabled         BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT pk_notification_preferences PRIMARY KEY (id),
    CONSTRAINT fk_notif_pref_org
        FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
    CONSTRAINT fk_notif_pref_user
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT uq_notification_preferences
        UNIQUE (organization_id, user_id, event_type, channel)
);

COMMENT ON TABLE notification_preferences IS
    'Per-user notification preferences within an organization. '
    'A missing row means notifications are enabled (opt-out model). '
    'Writes use UPSERT (last-writer-wins) on (organization_id, user_id, event_type, channel).';

-- Lookup index for DrainOutbox preference checks.
CREATE INDEX idx_notif_prefs_lookup
    ON notification_preferences (organization_id, user_id, event_type, channel);

COMMENT ON INDEX idx_notif_prefs_lookup IS
    'DrainOutbox preference check: quickly determine whether a user has opted out '
    'of a specific event_type / channel combination within an org.';


-- =============================================================================
-- SECTION 5: SEED notification.manage PERMISSION
-- =============================================================================
-- notification.manage grants access to administrative notification operations.
-- Personal notification endpoints (read, delete own) require only RequireAuth.

INSERT INTO permissions (name, slug, resource, action, description)
VALUES (
    'Manage Notifications',
    'notification.manage',
    'notification',
    'manage',
    'Manage notification settings and view all notifications for the organization'
)
ON CONFLICT (slug) DO NOTHING;

-- Grant to platform_admin, org_owner, org_admin.
-- scorers and viewers have no notification admin rights.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM (VALUES
    ('platform_admin', 'notification.manage'),
    ('org_owner',      'notification.manage'),
    ('org_admin',      'notification.manage')
) AS mapping(role_slug, perm_slug)
JOIN roles       r ON r.slug = mapping.role_slug AND r.organization_id IS NULL
JOIN permissions p ON p.slug = mapping.perm_slug
ON CONFLICT DO NOTHING;
