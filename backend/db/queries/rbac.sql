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
