-- =============================================================================
-- Migration  : 000005_create_players (UP)
-- Description: Creates the players table — org-scoped athletic profiles.
--              Deliberately decoupled from users: historical players and
--              scouted athletes may not have platform accounts.
-- Depends on : 000002 (organizations), 000003 (users), 000001 (player_status, gender ENUMs)
-- =============================================================================

CREATE TABLE players (
    id              UUID          NOT NULL DEFAULT gen_random_uuid(),
    organization_id UUID          NOT NULL,
    user_id         UUID,
    display_name    TEXT          NOT NULL,
    jersey_number   TEXT,
    position        TEXT,
    height_cm       SMALLINT,
    weight_kg       SMALLINT,
    dominant_hand   TEXT,
    nationality     CHAR(2),
    date_of_birth   DATE,
    status          player_status NOT NULL DEFAULT 'active',
    bio             TEXT,
    metadata        JSONB         NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),

    CONSTRAINT pk_players                  PRIMARY KEY (id),
    CONSTRAINT fk_players_organization     FOREIGN KEY (organization_id)
                                           REFERENCES organizations (id) ON DELETE CASCADE,
    CONSTRAINT fk_players_user             FOREIGN KEY (user_id)
                                           REFERENCES users (id)         ON DELETE SET NULL,
    CONSTRAINT chk_players_display_name    CHECK (char_length(trim(display_name)) >= 1),
    CONSTRAINT chk_players_height_cm       CHECK (height_cm IS NULL OR (height_cm > 50 AND height_cm < 300)),
    CONSTRAINT chk_players_weight_kg       CHECK (weight_kg IS NULL OR (weight_kg > 20 AND weight_kg < 300)),
    CONSTRAINT chk_players_dominant_hand   CHECK (
        dominant_hand IS NULL
        OR dominant_hand IN ('left', 'right', 'ambidextrous')
    ),
    CONSTRAINT chk_players_nationality     CHECK (nationality IS NULL OR nationality ~ '^[A-Z]{2}$'),
    CONSTRAINT chk_players_date_of_birth   CHECK (date_of_birth IS NULL OR date_of_birth < CURRENT_DATE)
);

COMMENT ON TABLE players IS
    'Athletic profile scoped to an organization. '
    'Decoupled from users: one user can have multiple player profiles across orgs, '
    'and historical/scouted players may have no platform account at all. '
    'ON DELETE CASCADE: removing an org removes all its player records. '
    'ON DELETE SET NULL on user_id: deleting a user account does not erase the player record.';

COMMENT ON COLUMN players.user_id IS
    'Links this player profile to a platform user account. '
    'NULL for unregistered or historical players. '
    'One user may have N player profiles (different orgs, different sports).';

COMMENT ON COLUMN players.display_name IS
    'Public-facing name shown on scoreboards, brackets, and profiles. '
    'May differ from the linked user first_name + last_name (e.g. a sporting alias).';

COMMENT ON COLUMN players.position IS
    'Sport-specific playing position. Free text to support all sports. '
    'Examples: raider, all-rounder, left-defender (kabaddi); striker (football).';

COMMENT ON COLUMN players.metadata IS
    'Sport-specific attributes that do not fit the fixed schema: '
    'raid style (kabaddi), batting order (cricket), preferred formation role, etc.';

COMMENT ON COLUMN players.nationality IS
    'ISO 3166-1 alpha-2 country code (e.g. IN, US). Two uppercase letters.';

-- ---------------------------------------------------------------------------
-- Indexes
-- ---------------------------------------------------------------------------

-- Primary lookup: players in an org
CREATE INDEX idx_players_organization_id ON players (organization_id);

-- Auth linkage: find player profile for a logged-in user within an org
CREATE INDEX idx_players_user_id         ON players (user_id) WHERE user_id IS NOT NULL;

-- Roster filtering: active/injured/suspended players
CREATE INDEX idx_players_status          ON players (status);
