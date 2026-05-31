-- =============================================================================
-- Migration  : 000013_create_audit_logs (UP)
-- Description: Creates the audit_logs table — an immutable, system-wide
--              ledger of significant mutations, authentication events, and
--              permission changes.
--              No UPDATE or DELETE is ever performed on this table.
--              organization_id is NULL for platform-level actions (super-admin
--              login, platform role grants).
--              user_id is NULL for system-initiated actions.
--
--              SCALING NOTE:
--              This table will grow unboundedly. When row count exceeds 50M,
--              add declarative range partitioning by created_at (monthly).
--              A future migration will handle this — do not pre-optimise now.
--
-- Depends on : 000002 (organizations), 000003 (users),
--              000001 (audit_action ENUM)
-- =============================================================================

CREATE TABLE audit_logs (
    id              UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id UUID,
    user_id         UUID,
    action          audit_action NOT NULL,
    entity_type     TEXT         NOT NULL,
    entity_id       UUID,
    old_data        JSONB,
    new_data        JSONB,
    ip_address      INET,
    user_agent      TEXT,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    -- No updated_at: audit_logs are immutable by design. No UPDATE is ever performed.

    CONSTRAINT pk_audit_logs                    PRIMARY KEY (id),
    CONSTRAINT fk_audit_organization            FOREIGN KEY (organization_id)
                                                REFERENCES organizations (id) ON DELETE SET NULL,
    CONSTRAINT fk_audit_user                    FOREIGN KEY (user_id)
                                                REFERENCES users (id)         ON DELETE SET NULL,
    CONSTRAINT chk_audit_entity_type_not_empty  CHECK (char_length(trim(entity_type)) >= 1),
    -- login / logout actions have no target entity; entity_id must be NULL
    CONSTRAINT chk_audit_login_has_no_entity    CHECK (
        action NOT IN ('login', 'logout') OR entity_id IS NULL
    ),
    -- create / update / delete / permission_change must reference a specific entity
    CONSTRAINT chk_audit_mutation_has_entity    CHECK (
        action IN ('login', 'logout') OR entity_id IS NOT NULL
    ),
    -- update actions must carry both before and after snapshots
    CONSTRAINT chk_audit_update_has_both_snapshots CHECK (
        action != 'update'
        OR (old_data IS NOT NULL AND new_data IS NOT NULL)
    )
);

COMMENT ON TABLE audit_logs IS
    'Immutable system-wide audit ledger. '
    'Every significant data mutation, login event, and permission change produces a row. '
    'No UPDATE or DELETE is ever performed here — rows are written once and never touched again. '
    'When row count exceeds 50M, add monthly range partitioning by created_at.';

COMMENT ON COLUMN audit_logs.organization_id IS
    'The org context in which the action occurred. '
    'NULL for platform-level actions (super-admin login, platform role grants). '
    'SET NULL on org deletion: audit records are retained regardless.';

COMMENT ON COLUMN audit_logs.user_id IS
    'The user who performed the action. '
    'NULL if the action was system-initiated (scheduled job, webhook, etc.). '
    'SET NULL on user deletion: audit records are retained regardless.';

COMMENT ON COLUMN audit_logs.entity_type IS
    'Table or domain name of the affected entity (e.g. tournaments, matches, players). '
    'Use the exact table name for consistency.';

COMMENT ON COLUMN audit_logs.old_data IS
    'JSONB snapshot of the entity state BEFORE an update or delete. '
    'NULL for create, login, and logout actions.';

COMMENT ON COLUMN audit_logs.new_data IS
    'JSONB snapshot of the entity state AFTER a create or update. '
    'NULL for delete, login, and logout actions.';

COMMENT ON COLUMN audit_logs.ip_address IS
    'INET type: stores both IPv4 and IPv6. '
    'Captured from the request context at write time.';

-- ---------------------------------------------------------------------------
-- Indexes
-- Audit queries are range-based (org + time window) and entity-based.
-- DESC ordering on created_at covers the "latest first" access pattern.
-- ---------------------------------------------------------------------------

-- Primary audit trail query: "show me the last N events for org X"
CREATE INDEX idx_audit_org_created_at ON audit_logs (organization_id, created_at DESC);

-- User-specific audit trail: "what did user X do?"
CREATE INDEX idx_audit_user_id        ON audit_logs (user_id)
    WHERE user_id IS NOT NULL;

-- Entity audit trail: "what happened to tournament X?"
CREATE INDEX idx_audit_entity         ON audit_logs (entity_type, entity_id)
    WHERE entity_id IS NOT NULL;

-- Action-type filter: "show all login events in the last 24 hours"
CREATE INDEX idx_audit_action         ON audit_logs (action, created_at DESC);
