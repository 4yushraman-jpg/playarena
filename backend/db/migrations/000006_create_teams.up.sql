-- =============================================================================
-- Migration  : 000006_create_teams (UP)
-- Description: Creates the teams table — org-scoped team entities.
--              slug is unique within an organization, not globally.
-- Depends on : 000002 (organizations), 000001 (team_status ENUM)
-- =============================================================================

CREATE TABLE teams (
    id              UUID        NOT NULL DEFAULT gen_random_uuid(),
    organization_id UUID        NOT NULL,
    name            TEXT        NOT NULL,
    short_name      TEXT,
    slug            TEXT        NOT NULL,
    description     TEXT,
    logo_url        TEXT,
    home_city       TEXT,
    home_venue      TEXT,
    founded_year    SMALLINT,
    primary_color   TEXT,
    secondary_color TEXT,
    status          team_status NOT NULL DEFAULT 'active',
    metadata        JSONB       NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT pk_teams                  PRIMARY KEY (id),
    CONSTRAINT uq_teams_org_slug         UNIQUE (organization_id, slug),
    CONSTRAINT fk_teams_organization     FOREIGN KEY (organization_id)
                                         REFERENCES organizations (id) ON DELETE CASCADE,
    CONSTRAINT chk_teams_slug            CHECK (
        slug ~ '^[a-z0-9][a-z0-9\-]*[a-z0-9]$'
        AND char_length(slug) BETWEEN 3 AND 100
    ),
    CONSTRAINT chk_teams_name            CHECK (
        char_length(trim(name)) >= 1
        AND char_length(name) <= 255
    ),
    CONSTRAINT chk_teams_short_name      CHECK (
        short_name IS NULL
        OR char_length(trim(short_name)) BETWEEN 2 AND 10
    ),
    CONSTRAINT chk_teams_founded_year    CHECK (
        founded_year IS NULL
        OR (founded_year >= 1800 AND founded_year <= 2100)
    ),
    CONSTRAINT chk_teams_primary_color   CHECK (
        primary_color IS NULL
        OR primary_color ~ '^#[0-9A-Fa-f]{6}$'
    ),
    CONSTRAINT chk_teams_secondary_color CHECK (
        secondary_color IS NULL
        OR secondary_color ~ '^#[0-9A-Fa-f]{6}$'
    )
);

COMMENT ON TABLE teams IS
    'A team entity scoped to an organization. slug is unique within the org. '
    'disbanded teams are retained permanently for historical match and ranking records: '
    'hard-deleting a disbanded team would corrupt match winner references.';

COMMENT ON COLUMN teams.short_name IS
    'Abbreviation for scoreboards and brackets (e.g. MUM for Mumbai Raiders). '
    'Constrained to 2–10 characters. Used where full team name does not fit.';

COMMENT ON COLUMN teams.primary_color IS
    'Hex color code for the primary kit color (e.g. #FF5733). '
    'Used in UI theming, bracket displays, and charts. Validated as #RRGGBB.';

COMMENT ON COLUMN teams.secondary_color IS
    'Hex color code for the secondary kit color. Same format as primary_color.';

COMMENT ON COLUMN teams.metadata IS
    'Sport-specific or org-specific team attributes. '
    'Examples: home_state, social_media_handles, kit_supplier, achievements.';

-- ---------------------------------------------------------------------------
-- Indexes
-- ---------------------------------------------------------------------------

-- Primary lookup: all teams in an org
CREATE INDEX idx_teams_organization_id ON teams (organization_id);

-- Status filter: active teams for registration / match scheduling
CREATE INDEX idx_teams_status          ON teams (status);
