-- =============================================================================
-- Migration  : 000015_auth_hardening (DOWN)
-- Description: Reverses auth hardening changes.
--              WARNING: will fail if any platform-grant rows (organization_id
--              IS NULL) exist, because restoring NOT NULL requires all rows to
--              be non-NULL. Remove or migrate those rows first.
-- =============================================================================

DROP INDEX IF EXISTS idx_uor_platform_grants;
DROP INDEX IF EXISTS uq_uor_platform_grant;

ALTER TABLE user_organization_roles
    ALTER COLUMN organization_id SET NOT NULL;
