-- RBAC queries
-- Role and permission lookups for authorization

-- ── existing queries (used by auth service login flow) ────────────────────────

-- name: GetUserRolesByOrganization :many
SELECT r.*
FROM   user_organization_roles uor
JOIN   roles r ON uor.role_id = r.id
WHERE  uor.user_id = $1
  AND  uor.organization_id = $2
  AND  (uor.expires_at IS NULL OR uor.expires_at > NOW())
ORDER  BY r.name ASC;

-- name: GetUserPlatformRoles :many
SELECT r.*
FROM   user_organization_roles uor
JOIN   roles r ON uor.role_id = r.id
WHERE  uor.user_id = $1
  AND  uor.organization_id IS NULL
  AND  (uor.expires_at IS NULL OR uor.expires_at > NOW())
ORDER  BY r.name ASC;

-- name: GetUserOrganizations :many
SELECT DISTINCT o.*
FROM   user_organization_roles uor
JOIN   organizations o ON uor.organization_id = o.id
WHERE  uor.user_id = $1
  AND  uor.organization_id IS NOT NULL
  AND  (uor.expires_at IS NULL OR uor.expires_at > NOW())
ORDER  BY o.name ASC;


-- ── authorization-layer queries ───────────────────────────────────────────────

-- name: GetUserRoles :many
-- Returns all active roles for a user, combining:
--   • org-specific grants   (uor.organization_id = $2)
--   • platform-level grants (uor.organization_id IS NULL)
-- Pass organization_id = NULL to retrieve only platform grants.
-- No N+1: single JOIN fetches roles in one round trip.
SELECT r.*
FROM   user_organization_roles uor
JOIN   roles r ON uor.role_id = r.id
WHERE  uor.user_id = $1
  AND  (uor.organization_id = $2 OR uor.organization_id IS NULL)
  AND  (uor.expires_at IS NULL OR uor.expires_at > NOW())
ORDER  BY r.name ASC;

-- name: GetUserPermissions :many
-- Returns all distinct permissions for a user, combining org-specific and
-- platform-level role grants. The four-table JOIN resolves the full chain
-- uor → role → role_permissions → permission in one query (no N+1).
-- Pass organization_id = NULL to evaluate platform grants only.
SELECT DISTINCT p.*
FROM   user_organization_roles uor
JOIN   roles r          ON uor.role_id = r.id
JOIN   role_permissions rp ON r.id = rp.role_id
JOIN   permissions p    ON rp.permission_id = p.id
WHERE  uor.user_id = $1
  AND  (uor.organization_id = $2 OR uor.organization_id IS NULL)
  AND  (uor.expires_at IS NULL OR uor.expires_at > NOW())
ORDER  BY p.slug ASC;

-- name: HasPermission :one
-- Returns TRUE when the user holds the given permission slug in the given
-- org context. Uses EXISTS for short-circuit evaluation — stops as soon as
-- one matching grant is found without fetching extra rows.
-- Pass organization_id = NULL to check platform-level grants only.
SELECT EXISTS(
    SELECT 1
    FROM   user_organization_roles uor
    JOIN   roles r          ON uor.role_id = r.id
    JOIN   role_permissions rp ON r.id = rp.role_id
    JOIN   permissions p    ON rp.permission_id = p.id
    WHERE  uor.user_id = $1
      AND  (uor.organization_id = $2 OR uor.organization_id IS NULL)
      AND  (uor.expires_at IS NULL OR uor.expires_at > NOW())
      AND  p.slug = $3
) AS has_permission;


-- ── role grant operations ─────────────────────────────────────────────────────

-- name: GetRoleBySlug :one
-- Looks up a system role by its slug. System roles (is_system=TRUE) always
-- have organization_id IS NULL — they are platform-wide templates assigned
-- to users within specific org contexts via user_organization_roles.
SELECT *
FROM   roles
WHERE  slug = $1
  AND  organization_id IS NULL
LIMIT  1;

-- name: GrantRoleToUserInOrg :exec
-- Grants a role to a user within a specific organization.
-- ON CONFLICT DO NOTHING: if the exact same grant already exists it is
-- silently skipped (idempotent).
INSERT INTO user_organization_roles (user_id, organization_id, role_id, granted_by)
VALUES ($1, $2, $3, $4)
ON CONFLICT DO NOTHING;

-- name: GrantOrgRole :exec
-- Grants a role to a user within a specific organization, with optional expiry.
-- ON CONFLICT DO NOTHING: idempotent — if the exact same (user, org, role)
-- grant already exists it is silently skipped.
INSERT INTO user_organization_roles (user_id, organization_id, role_id, granted_by, expires_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT DO NOTHING;

-- name: ListOrgMembersWithRoles :many
-- Returns all users with at least one active non-expired role in the org.
-- One row per (user × role) pair — aggregate into MemberResponse in Go.
SELECT
    u.id           AS user_id,
    u.email,
    u.username,
    u.first_name,
    u.last_name,
    u.status       AS user_status,
    r.slug         AS role_slug,
    r.name         AS role_name,
    uor.id         AS grant_id,
    uor.granted_at,
    uor.expires_at,
    uor.granted_by
FROM   user_organization_roles uor
JOIN   users u ON u.id = uor.user_id
JOIN   roles r ON r.id = uor.role_id
WHERE  uor.organization_id = sqlc.arg(org_id)
  AND  (uor.expires_at IS NULL OR uor.expires_at > NOW())
ORDER  BY u.email ASC, r.slug ASC;

-- name: GetUserGrantsInOrg :many
-- Returns all active non-expired role grants for a specific user in a specific org.
-- Used to build the per-member response and to return the result after granting.
SELECT
    uor.id         AS grant_id,
    r.slug         AS role_slug,
    r.name         AS role_name,
    uor.granted_at,
    uor.expires_at,
    uor.granted_by
FROM   user_organization_roles uor
JOIN   roles r ON r.id = uor.role_id
WHERE  uor.user_id         = sqlc.arg(user_id)
  AND  uor.organization_id = sqlc.arg(org_id)
  AND  (uor.expires_at IS NULL OR uor.expires_at > NOW())
ORDER  BY r.slug ASC;

-- name: RevokeRoleFromUserInOrg :execrows
-- Deletes a specific role grant by (user_id, org_id, role_slug).
-- Returns rows deleted: 1 = deleted, 0 = grant did not exist.
-- USING avoids a correlated subquery and keeps organization_id unambiguous.
DELETE FROM user_organization_roles
USING  roles
WHERE  user_organization_roles.user_id         = sqlc.arg(user_id)
  AND  user_organization_roles.organization_id = sqlc.arg(org_id)
  AND  user_organization_roles.role_id         = roles.id
  AND  roles.slug                              = sqlc.arg(role_slug)
  AND  roles.organization_id                  IS NULL;

-- name: CountActiveOrgOwnersByOrg :one
-- Count of distinct users with a non-expired org_owner grant in the org.
-- Used to enforce the last-owner guard before revoking an org_owner role.
SELECT COUNT(DISTINCT uor.user_id)
FROM   user_organization_roles uor
JOIN   roles r ON r.id = uor.role_id
WHERE  uor.organization_id = sqlc.arg(org_id)
  AND  r.slug              = 'org_owner'
  AND  (uor.expires_at IS NULL OR uor.expires_at > NOW());
