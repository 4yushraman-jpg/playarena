-- =============================================================================
-- Migration  : 000017_seed_rbac (DOWN)
-- Description: Removes all seeded RBAC data and reverts the constraint changes
--              introduced in the UP migration.
--
-- NOTE: This migration intentionally does NOT restore the original
-- chk_permissions_action constraint, because removing the 'upload' and
-- 'assign' actions while rows referencing them might still exist would
-- violate the constraint. In practice, running a full rollback to before
-- this migration implies a fresh database wipe.
-- =============================================================================

-- Remove role_permissions mappings for all system roles
DELETE FROM role_permissions
WHERE role_id IN (
    SELECT id FROM roles WHERE is_system = TRUE AND organization_id IS NULL
);

-- Remove all system roles seeded by this migration
DELETE FROM roles
WHERE is_system = TRUE
  AND organization_id IS NULL
  AND slug IN (
      'platform_admin', 'org_owner', 'org_admin',
      'team_manager', 'coach', 'scorer', 'viewer'
  );

-- Remove all permissions seeded by this migration
DELETE FROM permissions
WHERE slug IN (
    'organization.create', 'organization.update', 'organization.delete',
    'user.manage',
    'role.assign',
    'team.create',    'team.update',    'team.delete',
    'player.create',  'player.update',  'player.delete',
    'tournament.create', 'tournament.update', 'tournament.delete',
    'match.create',   'match.update',   'match.score',
    'media.upload'
);

-- Revert the roles constraint to the original (no system template exception)
ALTER TABLE roles DROP CONSTRAINT chk_roles_platform_no_org;

ALTER TABLE roles
    ADD CONSTRAINT chk_roles_platform_no_org CHECK (
        (scope = 'platform' AND organization_id IS NULL)
        OR (scope != 'platform' AND organization_id IS NOT NULL)
    );
