-- =============================================================================
-- Migration  : 000008_create_tournaments (DOWN)
-- Description: Drops the tournaments table.
--              Run AFTER rolling back 000009 (tournament_registrations),
--              000010 (matches), and 000011 (match_events).
-- =============================================================================

DROP TABLE IF EXISTS tournaments;
