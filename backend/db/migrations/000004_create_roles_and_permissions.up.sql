-- =============================================================================
-- Migration  : 000004_create_roles_and_permissions (UP)
-- Description: Creates the full RBAC layer:
--                permissions            — atomic capability definitions
--                roles                  — named permission groups (org or platform)
--                role_permissions       — M:M join (role ↔ permission)
--                user_organization_roles — grants a user a role within an org
-- Depends on : 000002 (organizations), 000003 (users), 000001 (role_scope ENUM)
-- =============================================================================

-- ---------------------------------------------------------------------------
-- permissions
-- Platform-level definitions. No organization_id: permissions are global.
-- Deployed by application migrations; never user-editable.
-- ---------------------------------------------------------------------------

CREATE TABLE permissions (
    id          UUID        NOT NULL DEFAULT gen_random_uuid(),
    name        TEXT        NOT NULL,
    slug        TEXT        NOT NULL,
    description TEXT,
    resource    TEXT        NOT NULL,
    action      TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- No updated_at: permission definitions are code-deployed, not runtime-editable.

    CONSTRAINT pk_permissions                  PRIMARY KEY (id),
    CONSTRAINT uq_permissions_slug             UNIQUE (slug),
    CONSTRAINT uq_permissions_resource_action  UNIQUE (resource, action),
    CONSTRAINT chk_permissions_slug            CHECK (slug ~ '^[a-z][a-z0-9_\.]*$'),
    CONSTRAINT chk_permissions_resource        CHECK (char_length(trim(resource)) >= 1),
    CONSTRAINT chk_permissions_action          CHECK (
        action IN ('create', 'read', 'update', 'delete', 'manage', 'score', 'approve', 'publish')
    )
);

COMMENT ON TABLE permissions IS
    'Atomic capability definitions. Deployed by application code; not user-editable. '
    'slug convention: <resource>.<action> (e.g. tournament.create, match.score). '
    'resource and action are kept as separate columns for fine-grained RBAC queries.';

COMMENT ON COLUMN permissions.slug IS
    'Dot-notation identifier used in application authorization checks. '
    'Format: <resource>.<action> (lowercase, underscores allowed).';

COMMENT ON COLUMN permissions.resource IS
    'Domain entity this permission applies to (e.g. tournament, match, player).';

COMMENT ON COLUMN permissions.action IS
    'Operation being permitted. Constrained to a known vocabulary by CHECK constraint.';


-- ---------------------------------------------------------------------------
-- roles
-- Can be platform-scoped (organization_id IS NULL) or org-scoped.
-- ---------------------------------------------------------------------------

CREATE TABLE roles (
    id              UUID        NOT NULL DEFAULT gen_random_uuid(),
    organization_id UUID,
    name            TEXT        NOT NULL,
    slug            TEXT        NOT NULL,
    description     TEXT,
    scope           role_scope  NOT NULL DEFAULT 'organization',
    is_system       BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT pk_roles                  PRIMARY KEY (id),
    CONSTRAINT uq_roles_org_slug         UNIQUE (organization_id, slug),
    CONSTRAINT fk_roles_organization     FOREIGN KEY (organization_id)
                                         REFERENCES organizations (id) ON DELETE CASCADE,
    CONSTRAINT chk_roles_platform_no_org CHECK (
        (scope = 'platform' AND organization_id IS NULL)
        OR (scope != 'platform' AND organization_id IS NOT NULL)
    ),
    CONSTRAINT chk_roles_slug            CHECK (slug ~ '^[a-z][a-z0-9_]*$'),
    CONSTRAINT chk_roles_name            CHECK (char_length(trim(name)) >= 1)
);

COMMENT ON TABLE roles IS
    'Named permission groups. '
    'Platform roles (scope = platform) have organization_id = NULL '
    'and are reserved for super-admins. '
    'Org roles are always paired with a non-NULL organization_id. '
    'is_system = TRUE marks roles seeded by migrations that cannot be deleted via the API.';

COMMENT ON COLUMN roles.organization_id IS
    'NULL for platform-scoped roles. Non-NULL for org-scoped roles. '
    'Enforced by chk_roles_platform_no_org.';

COMMENT ON COLUMN roles.is_system IS
    'Marks roles seeded by migrations (e.g. org_owner, scorer, viewer). '
    'The API must refuse to delete or rename system roles.';

CREATE INDEX idx_roles_organization_id ON roles (organization_id);
CREATE INDEX idx_roles_scope           ON roles (scope);


-- ---------------------------------------------------------------------------
-- role_permissions  (M:M join)
-- ---------------------------------------------------------------------------

CREATE TABLE role_permissions (
    role_id       UUID NOT NULL,
    permission_id UUID NOT NULL,

    CONSTRAINT pk_role_permissions            PRIMARY KEY (role_id, permission_id),
    CONSTRAINT fk_role_permissions_role       FOREIGN KEY (role_id)
                                              REFERENCES roles       (id) ON DELETE CASCADE,
    CONSTRAINT fk_role_permissions_permission FOREIGN KEY (permission_id)
                                              REFERENCES permissions (id) ON DELETE CASCADE
);

COMMENT ON TABLE role_permissions IS
    'Many-to-many join between roles and permissions. '
    'Assigning a role to a user grants all permissions linked to that role. '
    'CASCADE on both sides: deleting a role or permission cleans up the join rows.';

-- Reverse lookup: "which roles have this permission?"
CREATE INDEX idx_role_permissions_permission_id ON role_permissions (permission_id);


-- ---------------------------------------------------------------------------
-- user_organization_roles
-- A user can hold multiple roles within the same organization.
-- ---------------------------------------------------------------------------

CREATE TABLE user_organization_roles (
    id              UUID        NOT NULL DEFAULT gen_random_uuid(),
    user_id         UUID        NOT NULL,
    organization_id UUID        NOT NULL,
    role_id         UUID        NOT NULL,
    granted_by      UUID,
    granted_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT pk_user_organization_roles     PRIMARY KEY (id),
    CONSTRAINT uq_user_organization_roles     UNIQUE (user_id, organization_id, role_id),
    CONSTRAINT fk_uor_user                    FOREIGN KEY (user_id)
                                              REFERENCES users         (id) ON DELETE CASCADE,
    CONSTRAINT fk_uor_organization            FOREIGN KEY (organization_id)
                                              REFERENCES organizations (id) ON DELETE CASCADE,
    CONSTRAINT fk_uor_role                    FOREIGN KEY (role_id)
                                              REFERENCES roles         (id) ON DELETE RESTRICT,
    CONSTRAINT fk_uor_granted_by              FOREIGN KEY (granted_by)
                                              REFERENCES users         (id) ON DELETE SET NULL,
    CONSTRAINT chk_uor_expires_after_granted  CHECK (expires_at IS NULL OR expires_at > granted_at)
);

COMMENT ON TABLE user_organization_roles IS
    'Grants a user a role within a specific organization. '
    'A user can hold multiple roles in the same org (e.g. scorer + media_manager). '
    'ON DELETE RESTRICT on role_id: a role cannot be deleted while assigned to any user. '
    'Use this table as the authority for all authorization decisions.';

COMMENT ON COLUMN user_organization_roles.granted_by IS
    'The admin who granted this role assignment. '
    'SET NULL on user deletion: the grant record is retained regardless.';

COMMENT ON COLUMN user_organization_roles.expires_at IS
    'Optional expiry for time-limited grants (e.g. guest scorer for one tournament). '
    'The application must enforce this — no automatic revocation occurs at the DB layer.';

-- Auth hot path: "what roles does this user have in this org?"
CREATE INDEX idx_uor_user_id         ON user_organization_roles (user_id);
-- Admin panel: "who has access to this org?"
CREATE INDEX idx_uor_organization_id ON user_organization_roles (organization_id);
