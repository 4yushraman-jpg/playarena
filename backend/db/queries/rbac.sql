-- RBAC queries
-- Role and permission lookups for authorization

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
