-- =============================================================================
-- Migration  : 000008_create_tournaments (UP)
-- Description: Creates the tournaments table — the central scheduling entity.
--              sport is TEXT (not ENUM) to allow new sports without schema changes.
--              Format-specific configuration lives in the settings JSONB column.
-- Depends on : 000002 (organizations), 000003 (users),
--              000001 (tournament_status, tournament_format, participant_type ENUMs)
-- =============================================================================

CREATE TABLE tournaments (
    id                     UUID               NOT NULL DEFAULT gen_random_uuid(),
    organization_id        UUID               NOT NULL,
    name                   TEXT               NOT NULL,
    slug                   TEXT               NOT NULL,
    description            TEXT,
    sport                  TEXT               NOT NULL,
    format                 tournament_format  NOT NULL,
    participant_type       participant_type   NOT NULL DEFAULT 'team',
    status                 tournament_status  NOT NULL DEFAULT 'draft',
    banner_url             TEXT,
    prize_pool             NUMERIC(12, 2),
    currency               CHAR(3)            NOT NULL DEFAULT 'INR',
    max_participants       SMALLINT,
    min_participants       SMALLINT,
    registration_opens_at  TIMESTAMPTZ,
    registration_closes_at TIMESTAMPTZ,
    starts_at              TIMESTAMPTZ,
    ends_at                TIMESTAMPTZ,
    venue                  TEXT,
    city                   TEXT,
    country                CHAR(2),
    rules                  TEXT,
    settings               JSONB              NOT NULL DEFAULT '{}',
    created_by             UUID,
    created_at             TIMESTAMPTZ        NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ        NOT NULL DEFAULT NOW(),

    CONSTRAINT pk_tournaments                      PRIMARY KEY (id),
    CONSTRAINT uq_tournaments_org_slug             UNIQUE (organization_id, slug),
    CONSTRAINT fk_tournaments_organization         FOREIGN KEY (organization_id)
                                                   REFERENCES organizations (id) ON DELETE CASCADE,
    CONSTRAINT fk_tournaments_created_by           FOREIGN KEY (created_by)
                                                   REFERENCES users (id)         ON DELETE SET NULL,
    CONSTRAINT chk_tournaments_slug                CHECK (
        slug ~ '^[a-z0-9][a-z0-9\-]*[a-z0-9]$'
        AND char_length(slug) BETWEEN 3 AND 100
    ),
    CONSTRAINT chk_tournaments_name                CHECK (
        char_length(trim(name)) >= 2
        AND char_length(name) <= 255
    ),
    CONSTRAINT chk_tournaments_sport               CHECK (char_length(trim(sport)) >= 2),
    CONSTRAINT chk_tournaments_prize_pool          CHECK (prize_pool IS NULL OR prize_pool >= 0),
    CONSTRAINT chk_tournaments_currency            CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT chk_tournaments_participant_counts  CHECK (
        min_participants IS NULL
        OR max_participants IS NULL
        OR (min_participants > 0 AND max_participants >= min_participants)
    ),
    CONSTRAINT chk_tournaments_registration_window CHECK (
        registration_opens_at IS NULL
        OR registration_closes_at IS NULL
        OR registration_opens_at < registration_closes_at
    ),
    CONSTRAINT chk_tournaments_event_dates         CHECK (
        starts_at IS NULL
        OR ends_at IS NULL
        OR starts_at <= ends_at
    ),
    CONSTRAINT chk_tournaments_country             CHECK (country IS NULL OR country ~ '^[A-Z]{2}$')
);

COMMENT ON TABLE tournaments IS
    'A tournament hosted by an organization. slug is unique within the org. '
    'Status transitions are one-way in application logic: '
    'draft → registration_open → registration_closed → ongoing → completed. '
    'Any state → cancelled is always permitted.';

COMMENT ON COLUMN tournaments.sport IS
    'Free-text sport identifier (e.g. kabaddi, cricket, football, badminton). '
    'Intentionally TEXT rather than ENUM: adding a new sport requires no schema migration. '
    'Normalise casing at the application layer (lowercase, trimmed).';

COMMENT ON COLUMN tournaments.format IS
    'Structural format determining how brackets and standings are computed. '
    'Format-specific config (group sizes, legs, tiebreaker rules) lives in settings JSONB.';

COMMENT ON COLUMN tournaments.settings IS
    'Format-specific configuration validated at the application layer. '
    'Examples: {"groups": 4, "teams_per_group": 3, "tiebreaker": ["points","nrr"]} '
    'for group_knockout; {"legs": 2} for league; {"bo": 3} for best-of-3 knockout.';

COMMENT ON COLUMN tournaments.currency IS
    'ISO 4217 three-letter currency code (e.g. INR, USD). '
    'Validated as three uppercase letters.';

COMMENT ON COLUMN tournaments.created_by IS
    'User who created this tournament. '
    'SET NULL on user deletion: the tournament is retained regardless.';

-- ---------------------------------------------------------------------------
-- Indexes
-- ---------------------------------------------------------------------------

-- Org dashboard: all tournaments for an org
CREATE INDEX idx_tournaments_organization_id ON tournaments (organization_id);

-- Public listing / status machine transitions
CREATE INDEX idx_tournaments_status          ON tournaments (status);

-- Sport-filtered discovery ("find all kabaddi tournaments")
CREATE INDEX idx_tournaments_sport           ON tournaments (sport);

-- Calendar / upcoming events query
CREATE INDEX idx_tournaments_starts_at       ON tournaments (starts_at);
