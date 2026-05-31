-- =============================================================================
-- Migration  : 000007_create_team_memberships (UP)
-- Description: Creates the team_memberships table — the player ↔ team join.
--              No unique constraint on (team_id, player_id): a player can
--              rejoin a team after leaving, generating a new membership row.
--              Full history is preserved; application logic queries the most
--              recent active row.
-- Depends on : 000005 (players), 000006 (teams),
--              000001 (membership_role, membership_status ENUMs)
-- =============================================================================

CREATE TABLE team_memberships (
    id            UUID              NOT NULL DEFAULT gen_random_uuid(),
    team_id       UUID              NOT NULL,
    player_id     UUID              NOT NULL,
    role          membership_role   NOT NULL DEFAULT 'player',
    jersey_number TEXT,
    status        membership_status NOT NULL DEFAULT 'active',
    joined_at     TIMESTAMPTZ       NOT NULL DEFAULT NOW(),
    left_at       TIMESTAMPTZ,
    notes         TEXT,
    created_at    TIMESTAMPTZ       NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ       NOT NULL DEFAULT NOW(),

    CONSTRAINT pk_team_memberships                      PRIMARY KEY (id),
    CONSTRAINT fk_team_memberships_team                 FOREIGN KEY (team_id)
                                                        REFERENCES teams   (id) ON DELETE CASCADE,
    CONSTRAINT fk_team_memberships_player               FOREIGN KEY (player_id)
                                                        REFERENCES players (id) ON DELETE CASCADE,
    CONSTRAINT chk_team_memberships_left_at             CHECK (
        left_at IS NULL OR left_at >= joined_at
    ),
    CONSTRAINT chk_team_memberships_exit_requires_left_at CHECK (
        status NOT IN ('transferred', 'released') OR left_at IS NOT NULL
    )
);

COMMENT ON TABLE team_memberships IS
    'Tracks a player belonging to a team over a specific time window. '
    'No UNIQUE(team_id, player_id): players can rejoin teams, each stint is a new row. '
    'transferred and released statuses both require left_at to be set. '
    'Application logic determines the "current" membership by querying '
    'WHERE status = ''active'' ORDER BY joined_at DESC LIMIT 1.';

COMMENT ON COLUMN team_memberships.jersey_number IS
    'Overrides players.jersey_number for this team specifically. '
    'A player may wear different numbers across teams.';

COMMENT ON COLUMN team_memberships.role IS
    'Functional role within this team. captain/vice_captain are on-field designations '
    'and may be updated per tournament without changing the membership record.';

COMMENT ON COLUMN team_memberships.left_at IS
    'The date and time the player left this team. '
    'NULL means the player is currently an active member. '
    'Always set when transitioning to transferred or released.';

COMMENT ON COLUMN team_memberships.notes IS
    'Internal notes for the org admin (e.g. reason for release, loan details). '
    'Not shown to the public.';

-- ---------------------------------------------------------------------------
-- Indexes
-- ---------------------------------------------------------------------------

-- Hot path: "who is currently on this team?" (partial: active only)
CREATE INDEX idx_team_memberships_team_id   ON team_memberships (team_id)   WHERE status = 'active';

-- Player history: "which teams has this player been on?"
CREATE INDEX idx_team_memberships_player_id ON team_memberships (player_id);

-- Status filter for admin and reporting queries
CREATE INDEX idx_team_memberships_status    ON team_memberships (status);
