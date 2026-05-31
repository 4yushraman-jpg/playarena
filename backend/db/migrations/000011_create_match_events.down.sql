-- =============================================================================
-- Migration  : 000011_create_match_events (DOWN)
-- Description: Drops the match_events table and all its indexes.
--              This is the last migration in the core chain; run it first
--              when rolling back.
-- =============================================================================

DROP TABLE IF EXISTS match_events;
