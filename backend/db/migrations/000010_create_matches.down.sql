-- =============================================================================
-- Migration  : 000010_create_matches (DOWN)
-- Description: Drops the matches table.
--              Run AFTER rolling back 000011 (match_events).
-- =============================================================================

DROP TABLE IF EXISTS matches;
