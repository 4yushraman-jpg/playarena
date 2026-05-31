-- =============================================================================
-- Migration  : 000002_create_organizations (UP)
-- Description: Creates the organizations table — the multi-tenant root entity.
--              Every business-domain table carries organization_id as a FK here.
-- Depends on : 000001 (pgcrypto, org_status, org_type ENUMs)
-- =============================================================================

CREATE TABLE organizations (
    id          UUID        NOT NULL DEFAULT gen_random_uuid(),
    name        TEXT        NOT NULL,
    slug        TEXT        NOT NULL,
    description TEXT,
    type        org_type    NOT NULL DEFAULT 'independent',
    status      org_status  NOT NULL DEFAULT 'active',
    logo_url    TEXT,
    website     TEXT,
    email       TEXT,
    phone       TEXT,
    country     CHAR(2),
    city        TEXT,
    settings    JSONB       NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT pk_organizations          PRIMARY KEY (id),
    CONSTRAINT uq_organizations_slug     UNIQUE (slug),
    CONSTRAINT chk_organizations_slug    CHECK (
        slug ~ '^[a-z0-9][a-z0-9\-]*[a-z0-9]$'
        AND char_length(slug) BETWEEN 3 AND 100
    ),
    CONSTRAINT chk_organizations_name    CHECK (
        char_length(trim(name)) >= 2
        AND char_length(name) <= 255
    ),
    CONSTRAINT chk_organizations_country CHECK (country IS NULL OR country ~ '^[A-Z]{2}$'),
    CONSTRAINT chk_organizations_email   CHECK (email IS NULL OR email ~ '^[^@\s]+@[^@\s]+\.[^@\s]+$')
);

COMMENT ON TABLE organizations IS
    'Root multi-tenant entity. Every business-domain table carries organization_id as a FK. '
    'Deleting an organization cascades to all child records. '
    'Super-admin users exist outside any organization via platform-scoped roles.';

COMMENT ON COLUMN organizations.slug IS
    'URL-safe unique identifier. Lowercase alphanumeric and hyphens only. '
    'Used in public-facing API paths and routes. Immutable after creation.';

COMMENT ON COLUMN organizations.country IS
    'ISO 3166-1 alpha-2 country code (e.g. IN for India, US for United States). '
    'Two uppercase letters. Validated by chk_organizations_country.';

COMMENT ON COLUMN organizations.settings IS
    'Org-level configuration: feature flags, allowed sports, branding overrides. '
    'Schema-free to avoid migrations for per-org customisation. '
    'Structure is validated and documented at the application layer.';

-- ---------------------------------------------------------------------------
-- Indexes
-- ---------------------------------------------------------------------------

-- Status filter: "find all active organizations"
CREATE INDEX idx_organizations_status ON organizations (status);

-- Type filter: "find all federation-type organizations"
CREATE INDEX idx_organizations_type   ON organizations (type);
