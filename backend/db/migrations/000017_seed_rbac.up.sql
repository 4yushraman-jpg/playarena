-- =============================================================================
-- Migration  : 000017_seed_rbac (UP)
-- Description: Seeds all system roles, permissions, and role→permission
--              mappings required by the authorization layer.
--
-- CHANGES
-- ───────────────────────────────────────────────────────────────────────────
-- Section 1 — Constraint fixes
--   1A  Relax chk_roles_platform_no_org so system template roles may have
--       scope != 'platform' while still having organization_id = NULL.
--       These are the global role definitions used as templates; the actual
--       org context is carried by the user_organization_roles grant row.
--
--   1B  Extend chk_permissions_action to include the 'upload' and 'assign'
--       actions required by the media.upload and role.assign permissions.
--
-- Section 2 — Permissions (18 rows)
-- Section 3 — System roles (7 rows)
-- Section 4 — Role ↔ permission mappings
--
-- IDEMPOTENCY
-- ───────────────────────────────────────────────────────────────────────────
-- Every INSERT uses ON CONFLICT … DO NOTHING.
-- Running this migration twice is safe.
--
-- Depends on : 000004 (roles, permissions, role_permissions)
-- =============================================================================


-- =============================================================================
-- SECTION 1A: Relax the roles scope / org_id constraint
-- =============================================================================
-- Original constraint:
--   (scope = 'platform' AND org_id IS NULL)
--   OR (scope != 'platform' AND org_id IS NOT NULL)
--
-- New constraint additionally allows:
--   (scope != 'platform' AND org_id IS NULL AND is_system = TRUE)
--
-- This permits seeded "system template" roles (org_owner, scorer, etc.) to
-- exist without a specific organization_id. When these are granted to a user,
-- the user_organization_roles row carries the org context, not the role row.

ALTER TABLE roles DROP CONSTRAINT chk_roles_platform_no_org;

ALTER TABLE roles
    ADD CONSTRAINT chk_roles_platform_no_org CHECK (
        -- Classic: platform roles must have no org
        (scope = 'platform' AND organization_id IS NULL)
        OR
        -- Classic: org/tournament roles scoped to a specific org
        (scope != 'platform' AND organization_id IS NOT NULL)
        OR
        -- New: system template roles — org-scoped definitions without a specific org
        (scope != 'platform' AND organization_id IS NULL AND is_system = TRUE)
    );

COMMENT ON CONSTRAINT chk_roles_platform_no_org ON roles IS
    'Enforces org_id / scope consistency. '
    'Platform roles: scope=platform, org_id=NULL. '
    'Org roles: scope=organization, org_id=<uuid>. '
    'System template roles: scope=organization, org_id=NULL, is_system=TRUE — '
    'global permission templates assigned to users via user_organization_roles.';


-- =============================================================================
-- SECTION 1B: Extend the permissions action vocabulary
-- =============================================================================
-- 'upload' is needed for media.upload.
-- 'assign' is needed for role.assign.
-- All existing values are preserved.

ALTER TABLE permissions DROP CONSTRAINT chk_permissions_action;

ALTER TABLE permissions
    ADD CONSTRAINT chk_permissions_action CHECK (
        action IN (
            'create', 'read', 'update', 'delete',
            'manage', 'score', 'approve', 'publish',
            'upload', 'assign'
        )
    );

COMMENT ON CONSTRAINT chk_permissions_action ON permissions IS
    'Allowed action vocabulary. Extended in 000017 to include ''upload'' and ''assign''.';


-- =============================================================================
-- SECTION 2: PERMISSIONS
-- =============================================================================
-- Naming convention: <resource>.<action>
-- All 18 permissions required by the initial role set.
-- ON CONFLICT (slug) DO NOTHING makes this idempotent.

INSERT INTO permissions (name, slug, resource, action, description)
VALUES
    -- Organization management
    ('Create Organization',      'organization.create', 'organization', 'create', 'Create a new organization on the platform'),
    ('Update Organization',      'organization.update', 'organization', 'update', 'Edit organization profile, settings, and status'),
    ('Delete Organization',      'organization.delete', 'organization', 'delete', 'Permanently remove an organization and all its data'),

    -- User management
    ('Manage Users',             'user.manage',         'user',         'manage', 'Invite, deactivate, and manage user accounts within an organization'),

    -- Role management
    ('Assign Roles',             'role.assign',         'role',         'assign', 'Grant and revoke role assignments for users in an organization'),

    -- Team management
    ('Create Team',              'team.create',         'team',         'create', 'Create a new team within an organization'),
    ('Update Team',              'team.update',         'team',         'update', 'Edit team details, roster, and settings'),
    ('Delete Team',              'team.delete',         'team',         'delete', 'Disband a team and archive its history'),

    -- Player management
    ('Create Player',            'player.create',       'player',       'create', 'Register a new player profile within an organization'),
    ('Update Player',            'player.update',       'player',       'update', 'Edit player profile, stats, and eligibility status'),
    ('Delete Player',            'player.delete',       'player',       'delete', 'Remove a player profile from the organization'),

    -- Tournament management
    ('Create Tournament',        'tournament.create',   'tournament',   'create', 'Set up a new tournament'),
    ('Update Tournament',        'tournament.update',   'tournament',   'update', 'Edit tournament details, schedule, and settings'),
    ('Delete Tournament',        'tournament.delete',   'tournament',   'delete', 'Cancel and remove a tournament'),

    -- Match management
    ('Create Match',             'match.create',        'match',        'create', 'Schedule a new match fixture'),
    ('Update Match',             'match.update',        'match',        'update', 'Edit match details and status'),
    ('Score Match',              'match.score',         'match',        'score',  'Record live match events and scoring'),

    -- Media
    ('Upload Media',             'media.upload',        'media',        'upload', 'Upload images, videos, and documents')

ON CONFLICT (slug) DO NOTHING;


-- =============================================================================
-- SECTION 3: SYSTEM ROLES
-- =============================================================================
-- All roles have is_system = TRUE and organization_id = NULL.
-- Roles are identified by slug; idempotent via the partial unique index
-- uq_roles_platform_slug (slug) WHERE organization_id IS NULL.

-- platform_admin: full platform access (scope = platform)
INSERT INTO roles (name, slug, description, scope, is_system)
SELECT
    'Platform Administrator',
    'platform_admin',
    'Unrestricted access to all platform resources and settings. '
    'Reserved for PlayArena operators.',
    'platform',
    TRUE
WHERE NOT EXISTS (
    SELECT 1 FROM roles WHERE slug = 'platform_admin' AND organization_id IS NULL
);

-- org_owner: full organizational access (scope = organization, system template)
INSERT INTO roles (name, slug, description, scope, is_system)
SELECT
    'Organization Owner',
    'org_owner',
    'Full administrative access within a single organization. '
    'Typically assigned to the founding member who created the org.',
    'organization',
    TRUE
WHERE NOT EXISTS (
    SELECT 1 FROM roles WHERE slug = 'org_owner' AND organization_id IS NULL
);

-- org_admin: org management without nuclear options (scope = organization)
INSERT INTO roles (name, slug, description, scope, is_system)
SELECT
    'Organization Administrator',
    'org_admin',
    'Administrative access within an organization excluding destructive operations '
    '(org deletion, owner transfer). Suitable for delegated day-to-day management.',
    'organization',
    TRUE
WHERE NOT EXISTS (
    SELECT 1 FROM roles WHERE slug = 'org_admin' AND organization_id IS NULL
);

-- team_manager: team and player roster management (scope = organization)
INSERT INTO roles (name, slug, description, scope, is_system)
SELECT
    'Team Manager',
    'team_manager',
    'Manages team composition, player registrations, and squad logistics. '
    'Does not have access to tournament or platform settings.',
    'organization',
    TRUE
WHERE NOT EXISTS (
    SELECT 1 FROM roles WHERE slug = 'team_manager' AND organization_id IS NULL
);

-- coach: player updates and match participation (scope = organization)
INSERT INTO roles (name, slug, description, scope, is_system)
SELECT
    'Coach',
    'coach',
    'Updates player profiles and match details. '
    'Intended for coaching staff who need in-match control without full management access.',
    'organization',
    TRUE
WHERE NOT EXISTS (
    SELECT 1 FROM roles WHERE slug = 'coach' AND organization_id IS NULL
);

-- scorer: live match event recording (scope = organization)
INSERT INTO roles (name, slug, description, scope, is_system)
SELECT
    'Scorer',
    'scorer',
    'Records live match events and updates scores. '
    'No access to team, player, or tournament management.',
    'organization',
    TRUE
WHERE NOT EXISTS (
    SELECT 1 FROM roles WHERE slug = 'scorer' AND organization_id IS NULL
);

-- viewer: no write permissions — read-only access (scope = organization)
INSERT INTO roles (name, slug, description, scope, is_system)
SELECT
    'Viewer',
    'viewer',
    'Read-only access. Can view all organization data but cannot make any changes.',
    'organization',
    TRUE
WHERE NOT EXISTS (
    SELECT 1 FROM roles WHERE slug = 'viewer' AND organization_id IS NULL
);


-- =============================================================================
-- SECTION 4: ROLE → PERMISSION MAPPINGS
-- =============================================================================
-- Joins by slug so UUIDs never need to be hardcoded.
-- ON CONFLICT DO NOTHING makes this safe to re-run.

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM (VALUES
    -- ── platform_admin: full access to every permission ──────────────────────
    ('platform_admin', 'organization.create'),
    ('platform_admin', 'organization.update'),
    ('platform_admin', 'organization.delete'),
    ('platform_admin', 'user.manage'),
    ('platform_admin', 'role.assign'),
    ('platform_admin', 'team.create'),
    ('platform_admin', 'team.update'),
    ('platform_admin', 'team.delete'),
    ('platform_admin', 'player.create'),
    ('platform_admin', 'player.update'),
    ('platform_admin', 'player.delete'),
    ('platform_admin', 'tournament.create'),
    ('platform_admin', 'tournament.update'),
    ('platform_admin', 'tournament.delete'),
    ('platform_admin', 'match.create'),
    ('platform_admin', 'match.update'),
    ('platform_admin', 'match.score'),
    ('platform_admin', 'media.upload'),

    -- ── org_owner: full org access; cannot create new orgs (platform-only) ───
    ('org_owner', 'organization.update'),
    ('org_owner', 'organization.delete'),
    ('org_owner', 'user.manage'),
    ('org_owner', 'role.assign'),
    ('org_owner', 'team.create'),
    ('org_owner', 'team.update'),
    ('org_owner', 'team.delete'),
    ('org_owner', 'player.create'),
    ('org_owner', 'player.update'),
    ('org_owner', 'player.delete'),
    ('org_owner', 'tournament.create'),
    ('org_owner', 'tournament.update'),
    ('org_owner', 'tournament.delete'),
    ('org_owner', 'match.create'),
    ('org_owner', 'match.update'),
    ('org_owner', 'match.score'),
    ('org_owner', 'media.upload'),

    -- ── org_admin: day-to-day management; no org deletion ────────────────────
    ('org_admin', 'organization.update'),
    ('org_admin', 'user.manage'),
    ('org_admin', 'role.assign'),
    ('org_admin', 'team.create'),
    ('org_admin', 'team.update'),
    ('org_admin', 'team.delete'),
    ('org_admin', 'player.create'),
    ('org_admin', 'player.update'),
    ('org_admin', 'player.delete'),
    ('org_admin', 'tournament.create'),
    ('org_admin', 'tournament.update'),
    ('org_admin', 'tournament.delete'),
    ('org_admin', 'match.create'),
    ('org_admin', 'match.update'),
    ('org_admin', 'match.score'),
    ('org_admin', 'media.upload'),

    -- ── team_manager: team and player roster management ───────────────────────
    ('team_manager', 'team.create'),
    ('team_manager', 'team.update'),
    ('team_manager', 'team.delete'),
    ('team_manager', 'player.create'),
    ('team_manager', 'player.update'),
    ('team_manager', 'player.delete'),
    ('team_manager', 'media.upload'),

    -- ── coach: player updates + match participation ───────────────────────────
    ('coach', 'player.update'),
    ('coach', 'match.update'),
    ('coach', 'media.upload'),

    -- ── scorer: live scoring only ────────────────────────────────────────────
    ('scorer', 'match.update'),
    ('scorer', 'match.score')

    -- viewer: no write permissions — intentionally empty

) AS mapping(role_slug, perm_slug)
JOIN roles r       ON r.slug = mapping.role_slug AND r.organization_id IS NULL
JOIN permissions p ON p.slug = mapping.perm_slug
ON CONFLICT DO NOTHING;
