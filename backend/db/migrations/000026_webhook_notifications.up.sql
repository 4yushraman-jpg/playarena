-- =============================================================================
-- Migration  : 000026_webhook_notifications (UP)
-- Description: Adds webhook endpoint registry and webhook delivery queue for
--              Phase 19 Webhook Notification Delivery.
--
-- Schema overview:
--   webhook_endpoints — registered URLs per organization.
--     Secret stored AES-256-GCM encrypted; raw secret shown once on creation.
--
--   webhook_deliveries — delivery queue (endpoint-centric fan-out).
--     One row per (outbox_id, endpoint_id). State machine mirrors
--     notifications email delivery (Phase 25).
--
-- Delivery state machine:
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
-- HTTP 4xx (except 429) → immediate dead-letter (client bug, no point retrying).
--
-- Depends on : 000022_notifications (notification_event_type ENUM)
-- =============================================================================

-- ── webhook_endpoints ─────────────────────────────────────────────────────────

CREATE TABLE webhook_endpoints (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID        NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,

    -- HTTPS URL validated at registration time. Stored as-is (no normalisation).
    url             TEXT        NOT NULL,

    -- AES-256-GCM ciphertext of the raw webhook secret.
    -- Format: 12-byte nonce || ciphertext (standard GCM prefix).
    -- Decrypted only by the WebhookWorker at delivery time to compute HMAC.
    -- The raw secret is shown to the caller once on creation and never again.
    secret_ciphertext BYTEA     NOT NULL,

    -- Human-readable label (optional). Not used by the delivery system.
    description     TEXT,

    -- When FALSE the endpoint is registered but receives no deliveries.
    -- Useful for temporarily pausing without deleting.
    active          BOOLEAN     NOT NULL DEFAULT TRUE,

    created_by      UUID        NOT NULL REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE webhook_endpoints IS
    'Registered webhook endpoints per organization. '
    'SSRF-safe: URL is validated at registration time and re-validated at delivery.';

COMMENT ON COLUMN webhook_endpoints.secret_ciphertext IS
    'AES-256-GCM encrypted webhook secret (nonce prepended). '
    'Raw secret is returned once on creation, stored encrypted, never exposed again.';

-- Scoped index: list active endpoints for an org (used by DrainOutbox fan-out).
CREATE INDEX idx_webhook_endpoints_org_active
    ON webhook_endpoints (organization_id)
    WHERE active = TRUE;

-- ── webhook_deliveries ────────────────────────────────────────────────────────

CREATE TABLE webhook_deliveries (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID        NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    endpoint_id     UUID        NOT NULL REFERENCES webhook_endpoints(id) ON DELETE CASCADE,
    outbox_id       UUID        NOT NULL REFERENCES notification_outbox(id) ON DELETE CASCADE,

    -- Denormalised from outbox entry for efficient worker queries without joins.
    event_type      notification_event_type NOT NULL,
    entity_type     TEXT        NOT NULL,
    entity_id       UUID        NOT NULL,
    payload         JSONB       NOT NULL,

    -- Delivery state (mirrors notifications email delivery columns).
    attempt_count       INT         NOT NULL DEFAULT 0,
    last_attempted_at   TIMESTAMPTZ,
    lease_expires_at    TIMESTAMPTZ,
    sent_at             TIMESTAMPTZ,
    failed_permanently  BOOLEAN     NOT NULL DEFAULT FALSE,

    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE webhook_deliveries IS
    'Endpoint-centric delivery queue for webhook notifications. '
    'One row per (outbox_id, endpoint_id). Fan-out performed by DrainOutbox.';

-- Idempotency guard: DrainOutbox may be retried; this prevents duplicate rows.
CREATE UNIQUE INDEX uq_webhook_deliveries_outbox_endpoint
    ON webhook_deliveries (outbox_id, endpoint_id);

-- Worker claim index: finds eligible pending rows for delivery.
CREATE INDEX idx_webhook_deliveries_pending
    ON webhook_deliveries (last_attempted_at ASC NULLS FIRST, created_at ASC)
    WHERE sent_at IS NULL
      AND failed_permanently = FALSE;

COMMENT ON INDEX idx_webhook_deliveries_pending IS
    'WebhookWorker claim index: finds eligible rows (not sent, not dead-lettered) '
    'in oldest-first order. lease_expires_at < NOW() applied as heap filter.';

-- ── RBAC: webhook permissions ─────────────────────────────────────────────────
-- Seeds webhook.{create,read,update,delete} permissions and grants them to
-- org_owner, org_admin, and platform_admin roles.
-- ON CONFLICT DO NOTHING makes this idempotent.

INSERT INTO permissions (name, slug, resource, action, description)
VALUES
    ('Create Webhook',  'webhook.create', 'webhook', 'create', 'Register a new webhook endpoint for the organization'),
    ('Read Webhooks',   'webhook.read',   'webhook', 'read',   'List and view webhook endpoints for the organization'),
    ('Update Webhook',  'webhook.update', 'webhook', 'update', 'Toggle active/inactive on webhook endpoints'),
    ('Delete Webhook',  'webhook.delete', 'webhook', 'delete', 'Remove a webhook endpoint registration')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM (VALUES
    ('platform_admin', 'webhook.create'),
    ('platform_admin', 'webhook.read'),
    ('platform_admin', 'webhook.update'),
    ('platform_admin', 'webhook.delete'),
    ('org_owner',      'webhook.create'),
    ('org_owner',      'webhook.read'),
    ('org_owner',      'webhook.update'),
    ('org_owner',      'webhook.delete'),
    ('org_admin',      'webhook.create'),
    ('org_admin',      'webhook.read'),
    ('org_admin',      'webhook.update'),
    ('org_admin',      'webhook.delete')
) AS x(role_slug, perm_slug)
JOIN roles r       ON r.slug = x.role_slug AND r.organization_id IS NULL
JOIN permissions p ON p.slug = x.perm_slug
ON CONFLICT DO NOTHING;
