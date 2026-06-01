-- =============================================================================
-- Migration  : 000018_match_delete_permission (UP)
-- Description: Seeds the missing match.delete permission and grants it to
--              admin-level roles only (platform_admin, org_owner, org_admin).
--
-- BACKGROUND
-- ───────────────────────────────────────────────────────────────────────────
-- Migration 000017 seeded three match-resource permissions:
--   match.create, match.update, match.score
--
-- match.delete was omitted. Without it, the only way to guard the match
-- cancellation (DELETE) endpoint is to reuse match.update, which is also
-- granted to coach and scorer. That would allow coaching staff and scorers to
-- cancel match fixtures — an unintended privilege escalation relative to both
-- role descriptions and the pattern used by every other resource
-- (organization.delete, team.delete, player.delete, tournament.delete are all
-- restricted to admin-level roles).
--
-- This migration adds the missing permission and grants it to exactly the
-- same role set as tournament.delete: platform_admin, org_owner, org_admin.
--
-- GRANT MATRIX AFTER THIS MIGRATION
-- ───────────────────────────────────────────────────────────────────────────
--   match.create  → platform_admin, org_owner, org_admin
--   match.update  → platform_admin, org_owner, org_admin, coach, scorer
--   match.delete  → platform_admin, org_owner, org_admin              (new)
--   match.score   → platform_admin, org_owner, org_admin, scorer
--
-- IDEMPOTENCY
-- ───────────────────────────────────────────────────────────────────────────
-- ON CONFLICT … DO NOTHING on both INSERT statements. Safe to re-run.
--
-- Depends on : 000017 (permissions table, role slugs, role_permissions table)
-- =============================================================================


-- -----------------------------------------------------------------------------
-- Step 1: Seed the match.delete permission row
-- -----------------------------------------------------------------------------

INSERT INTO permissions (name, slug, resource, action, description)
VALUES (
    'Delete Match',
    'match.delete',
    'match',
    'delete',
    'Cancel a scheduled match fixture. Restricted to administrators; '
    'must not be granted to operational roles (coach, scorer).'
)
ON CONFLICT (slug) DO NOTHING;


-- -----------------------------------------------------------------------------
-- Step 2: Grant match.delete to platform_admin, org_owner, org_admin
-- -----------------------------------------------------------------------------
-- Mirrors the tournament.delete grant set exactly.
-- Joined by slug so UUIDs are never hardcoded.

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM (VALUES
    ('platform_admin', 'match.delete'),
    ('org_owner',      'match.delete'),
    ('org_admin',      'match.delete')
) AS mapping(role_slug, perm_slug)
JOIN roles       r ON r.slug = mapping.role_slug AND r.organization_id IS NULL
JOIN permissions p ON p.slug = mapping.perm_slug
ON CONFLICT DO NOTHING;
