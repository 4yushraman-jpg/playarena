-- =============================================================================
-- Migration  : 000015_auth_hardening (UP)
-- Description: Fixes two auth-layer schema defects discovered during Phase 3.5:
--
--   1. user_organization_roles.organization_id was NOT NULL, making it
--      impossible to store platform-scoped role grants (super-admin users).
--      Platform roles in the roles table correctly have organization_id = NULL
--      (enforced by chk_roles_platform_no_org), but the grant record had no
--      matching NULL path. This migration drops the NOT NULL constraint.
--
--   2. PostgreSQL UNIQUE constraints treat NULLs as distinct from each other,
--      so the existing uq_user_organization_roles(user_id, organization_id, role_id)
--      would allow duplicate platform grants for the same (user_id, role_id)
--      when organization_id IS NULL. A partial unique index covers that gap.
--
-- Depends on : 000004 (user_organization_roles)
-- =============================================================================

-- ---------------------------------------------------------------------------
-- 1. Allow NULL organization_id for platform-scoped role grants.
--    The FK constraint on organization_id is preserved: NULL passes FK checks.
-- ---------------------------------------------------------------------------
ALTER TABLE user_organization_roles
    ALTER COLUMN organization_id DROP NOT NULL;

-- ---------------------------------------------------------------------------
-- 2. Prevent duplicate platform grants.
--    Standard UNIQUE treats NULLs as distinct — (user_id, NULL, role_id) does
--    not conflict with another (user_id, NULL, role_id). This partial index
--    provides the uniqueness guarantee for platform-scope grants specifically.
-- ---------------------------------------------------------------------------
CREATE UNIQUE INDEX uq_uor_platform_grant
    ON user_organization_roles (user_id, role_id)
    WHERE organization_id IS NULL;

-- ---------------------------------------------------------------------------
-- 3. Index for platform-grant lookups on the auth hot path.
-- ---------------------------------------------------------------------------
CREATE INDEX idx_uor_platform_grants
    ON user_organization_roles (user_id)
    WHERE organization_id IS NULL;
