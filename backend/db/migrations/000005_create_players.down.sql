-- =============================================================================
-- Migration  : 000005_create_players (DOWN)
-- Description: Drops the players table.
--              Run AFTER rolling back 000007 (team_memberships),
--              000009 (tournament_registrations), 000010 (matches),
--              and 000011 (match_events).
-- =============================================================================

DROP TABLE IF EXISTS players;
