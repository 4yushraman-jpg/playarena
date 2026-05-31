-- =============================================================================
-- Migration  : 000004_create_roles_and_permissions (DOWN)
-- Description: Drops RBAC tables in reverse FK dependency order.
--              user_organization_roles must be dropped before roles.
--              role_permissions must be dropped before both roles and permissions.
-- =============================================================================

DROP TABLE IF EXISTS user_organization_roles;
DROP TABLE IF EXISTS role_permissions;
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS permissions;
