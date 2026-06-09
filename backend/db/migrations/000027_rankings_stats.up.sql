-- =============================================================================
-- Migration  : 000027_rankings_stats (UP)
-- Description: Adds per-tournament participation stats tables for Phase 22
--              Rankings. Two tables mirror each other — one for player-based
--              (individual) tournaments, one for team-based tournaments.
--
-- Design notes:
--   • organization_id is always the tournament HOST's org.  Cross-org
--     participants appear in the host org's rankings, not their own.
--   • Rows are upserted (ON CONFLICT DO UPDATE) at tournament completion,
--     so a retry after a crash is idempotent.
--   • Rankings are derived at read time via SQL GROUP BY + RANK().
--     Per-tournament rows (rather than a running aggregate) make the upsert
--     correct by construction and enable future date-range filtering.
--   • No CASCADE from tournaments — intentional.  If a tournament is later
--     hard-deleted (not current behaviour), the stats orphan is harmless and
--     the FK violation prevents accidental data loss.
-- =============================================================================

CREATE TABLE player_tournament_stats (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    player_id       UUID        NOT NULL REFERENCES players(id),
    tournament_id   UUID        NOT NULL REFERENCES tournaments(id),
    organization_id UUID        NOT NULL REFERENCES organizations(id),

    position        INT         NOT NULL CHECK (position >= 1),
    matches_played  INT         NOT NULL DEFAULT 0 CHECK (matches_played >= 0),
    matches_won     INT         NOT NULL DEFAULT 0 CHECK (matches_won >= 0),
    matches_drawn   INT         NOT NULL DEFAULT 0 CHECK (matches_drawn >= 0),
    matches_lost    INT         NOT NULL DEFAULT 0 CHECK (matches_lost >= 0),
    points          INT         NOT NULL DEFAULT 0,
    score_for       INT         NOT NULL DEFAULT 0 CHECK (score_for >= 0),
    score_against   INT         NOT NULL DEFAULT 0 CHECK (score_against >= 0),

    snapshotted_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_player_tournament_stats UNIQUE (player_id, tournament_id)
);

COMMENT ON TABLE player_tournament_stats IS
    'Final standings result for one player in one completed tournament. '
    'Snapshotted at tournament completion. Source for all-time player rankings.';

-- org + player covering index: drives the rankings list query
CREATE INDEX idx_player_stats_org_player
    ON player_tournament_stats (organization_id, player_id);

-- tournament index: supports lookups by tournament (e.g. admin tools)
CREATE INDEX idx_player_stats_tournament
    ON player_tournament_stats (tournament_id);

-- =============================================================================

CREATE TABLE team_tournament_stats (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id         UUID        NOT NULL REFERENCES teams(id),
    tournament_id   UUID        NOT NULL REFERENCES tournaments(id),
    organization_id UUID        NOT NULL REFERENCES organizations(id),

    position        INT         NOT NULL CHECK (position >= 1),
    matches_played  INT         NOT NULL DEFAULT 0 CHECK (matches_played >= 0),
    matches_won     INT         NOT NULL DEFAULT 0 CHECK (matches_won >= 0),
    matches_drawn   INT         NOT NULL DEFAULT 0 CHECK (matches_drawn >= 0),
    matches_lost    INT         NOT NULL DEFAULT 0 CHECK (matches_lost >= 0),
    points          INT         NOT NULL DEFAULT 0,
    score_for       INT         NOT NULL DEFAULT 0 CHECK (score_for >= 0),
    score_against   INT         NOT NULL DEFAULT 0 CHECK (score_against >= 0),

    snapshotted_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_team_tournament_stats UNIQUE (team_id, tournament_id)
);

COMMENT ON TABLE team_tournament_stats IS
    'Final standings result for one team in one completed tournament. '
    'Snapshotted at tournament completion. Source for all-time team rankings.';

CREATE INDEX idx_team_stats_org_team
    ON team_tournament_stats (organization_id, team_id);

CREATE INDEX idx_team_stats_tournament
    ON team_tournament_stats (tournament_id);
