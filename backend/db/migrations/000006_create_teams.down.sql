-- =============================================================================
-- Migration  : 000006_create_teams (DOWN)
-- Description: Drops the teams table.
--              Run AFTER rolling back 000007 (team_memberships),
--              000009 (tournament_registrations), 000010 (matches),
--              and 000011 (match_events).
-- =============================================================================

DROP TABLE IF EXISTS teams;
