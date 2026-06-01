-- =============================================================================
-- Migration  : 000019_match_score_snapshots (DOWN)
-- =============================================================================

ALTER TABLE matches
    DROP COLUMN IF EXISTS home_score,
    DROP COLUMN IF EXISTS away_score;
