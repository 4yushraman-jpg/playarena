-- =============================================================================
-- Migration  : 000030_tournament_visibility (DOWN)
-- =============================================================================

DROP INDEX IF EXISTS idx_tournaments_public_lookup;
ALTER TABLE tournaments DROP COLUMN IF EXISTS visibility;
DROP TYPE IF EXISTS tournament_visibility;
