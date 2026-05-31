-- =============================================================================
-- Migration  : 000010_create_matches (UP)
-- Description: Creates the matches table — individual fixture rows within a
--              tournament.
--              organization_id is DENORMALIZED from tournaments for query
--              performance (avoids a JOIN on every match-list query).
--              winner_team_id / winner_player_id are acceptable denormalization:
--              they represent a final-state conclusion, not a running statistic.
--              Participant columns may be NULL for bracket matches whose
--              opponents are not yet determined (e.g. "Winner of Match 3 vs
--              Winner of Match 4").
-- Depends on : 000002 (organizations), 000005 (players), 000006 (teams),
--              000008 (tournaments), 000001 (match_status ENUM)
-- =============================================================================

CREATE TABLE matches (
    id               UUID         NOT NULL DEFAULT gen_random_uuid(),
    tournament_id    UUID         NOT NULL,
    organization_id  UUID         NOT NULL,
    round_number     SMALLINT,
    round_name       TEXT,
    match_number     SMALLINT,
    home_team_id     UUID,
    away_team_id     UUID,
    home_player_id   UUID,
    away_player_id   UUID,
    venue            TEXT,
    scheduled_at     TIMESTAMPTZ,
    started_at       TIMESTAMPTZ,
    ended_at         TIMESTAMPTZ,
    status           match_status NOT NULL DEFAULT 'scheduled',
    winner_team_id   UUID,
    winner_player_id UUID,
    is_walkover      BOOLEAN      NOT NULL DEFAULT FALSE,
    notes            TEXT,
    metadata         JSONB        NOT NULL DEFAULT '{}',
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    CONSTRAINT pk_matches                             PRIMARY KEY (id),
    CONSTRAINT fk_matches_tournament                  FOREIGN KEY (tournament_id)
                                                      REFERENCES tournaments (id) ON DELETE CASCADE,
    CONSTRAINT fk_matches_organization                FOREIGN KEY (organization_id)
                                                      REFERENCES organizations (id),
    CONSTRAINT fk_matches_home_team                   FOREIGN KEY (home_team_id)
                                                      REFERENCES teams    (id)   ON DELETE SET NULL,
    CONSTRAINT fk_matches_away_team                   FOREIGN KEY (away_team_id)
                                                      REFERENCES teams    (id)   ON DELETE SET NULL,
    CONSTRAINT fk_matches_home_player                 FOREIGN KEY (home_player_id)
                                                      REFERENCES players  (id)   ON DELETE SET NULL,
    CONSTRAINT fk_matches_away_player                 FOREIGN KEY (away_player_id)
                                                      REFERENCES players  (id)   ON DELETE SET NULL,
    CONSTRAINT fk_matches_winner_team                 FOREIGN KEY (winner_team_id)
                                                      REFERENCES teams    (id)   ON DELETE SET NULL,
    CONSTRAINT fk_matches_winner_player               FOREIGN KEY (winner_player_id)
                                                      REFERENCES players  (id)   ON DELETE SET NULL,

    -- Participants must be either both teams, both players, or both NULL (TBD bracket slot)
    CONSTRAINT chk_matches_participants               CHECK (
        (home_team_id IS NOT NULL AND away_team_id IS NOT NULL
            AND home_player_id IS NULL AND away_player_id IS NULL)
        OR (home_player_id IS NOT NULL AND away_player_id IS NOT NULL
            AND home_team_id IS NULL AND away_team_id IS NULL)
        OR (home_team_id IS NULL AND away_team_id IS NULL
            AND home_player_id IS NULL AND away_player_id IS NULL)
    ),
    CONSTRAINT chk_matches_ended_after_started        CHECK (
        ended_at IS NULL OR started_at IS NULL OR ended_at > started_at
    ),
    -- A walkover must declare a winner
    CONSTRAINT chk_matches_walkover_has_winner        CHECK (
        NOT is_walkover
        OR (winner_team_id IS NOT NULL OR winner_player_id IS NOT NULL)
    ),
    -- Winner must be one of the match participants (NULL comparisons evaluate to UNKNOWN,
    -- which satisfies the CHECK — safe for TBD bracket matches)
    CONSTRAINT chk_matches_winner_is_team_participant CHECK (
        winner_team_id IS NULL
        OR winner_team_id = home_team_id
        OR winner_team_id = away_team_id
    ),
    CONSTRAINT chk_matches_winner_is_player_participant CHECK (
        winner_player_id IS NULL
        OR winner_player_id = home_player_id
        OR winner_player_id = away_player_id
    )
);

COMMENT ON TABLE matches IS
    'Individual match fixture within a tournament. '
    'organization_id is denormalized from tournaments.organization_id to avoid '
    'a JOIN on every match-list query — must always equal the parent tournament org. '
    'Participant columns are NULL for bracket matches whose opponents are TBD. '
    'winner_* columns are set at completion: they are a final-state conclusion, '
    'not a running statistic, so denormalization is justified here.';

COMMENT ON COLUMN matches.organization_id IS
    'Denormalized from tournaments.organization_id. '
    'The application must keep this in sync with the parent tournament.';

COMMENT ON COLUMN matches.round_number IS
    'Numeric round index (1, 2, 3…). Used for sorting and bracket logic. '
    'Use round_name for display strings (Quarter Final, Semi Final, Final).';

COMMENT ON COLUMN matches.match_number IS
    'Sequential match identifier within the tournament. '
    'Determines display ordering on the fixture list.';

COMMENT ON COLUMN matches.metadata IS
    'Sport-specific match configuration: half duration in seconds, '
    'max timeouts per team, max substitutions, super raid rule variant, etc.';

COMMENT ON COLUMN matches.is_walkover IS
    'TRUE when one side did not appear. The winner is awarded without match_events. '
    'chk_matches_walkover_has_winner enforces that winner_* must be set.';

-- ---------------------------------------------------------------------------
-- Indexes
-- ---------------------------------------------------------------------------

-- Fixture list for a tournament
CREATE INDEX idx_matches_tournament_id   ON matches (tournament_id);

-- Org-level match dashboard
CREATE INDEX idx_matches_organization_id ON matches (organization_id);

-- Live scoreboard filter ("show me all live matches")
CREATE INDEX idx_matches_status          ON matches (status);

-- Calendar / upcoming fixtures
CREATE INDEX idx_matches_scheduled_at    ON matches (scheduled_at);

-- Team fixture history (partial: skip NULL participant slots)
CREATE INDEX idx_matches_home_team_id    ON matches (home_team_id)   WHERE home_team_id   IS NOT NULL;
CREATE INDEX idx_matches_away_team_id    ON matches (away_team_id)   WHERE away_team_id   IS NOT NULL;

-- Individual player fixture history
CREATE INDEX idx_matches_home_player_id  ON matches (home_player_id) WHERE home_player_id IS NOT NULL;
CREATE INDEX idx_matches_away_player_id  ON matches (away_player_id) WHERE away_player_id IS NOT NULL;
