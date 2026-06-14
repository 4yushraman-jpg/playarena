-- =============================================================================
-- Migration  : 000029_bracket_progression (DOWN)
-- Description: Reverses 000029. Drops the bracket edge / group_label columns and
--              restores the strict participant constraint (both-or-neither).
--              NOTE: the down migration will fail if any partially filled match
--              rows exist, which is correct — they are invalid under the strict
--              constraint and must be resolved before reverting.
-- =============================================================================

DROP INDEX IF EXISTS idx_matches_next_match_id;

ALTER TABLE matches
    DROP CONSTRAINT IF EXISTS chk_matches_no_self_next,
    DROP CONSTRAINT IF EXISTS chk_matches_next_slot,
    DROP CONSTRAINT IF EXISTS fk_matches_next_match;

ALTER TABLE matches
    DROP COLUMN IF EXISTS group_label,
    DROP COLUMN IF EXISTS next_match_slot,
    DROP COLUMN IF EXISTS next_match_id;

ALTER TABLE matches DROP CONSTRAINT chk_matches_participants;

ALTER TABLE matches ADD CONSTRAINT chk_matches_participants CHECK (
    (home_team_id IS NOT NULL AND away_team_id IS NOT NULL
        AND home_player_id IS NULL AND away_player_id IS NULL)
    OR (home_player_id IS NOT NULL AND away_player_id IS NOT NULL
        AND home_team_id IS NULL AND away_team_id IS NULL)
    OR (home_team_id IS NULL AND away_team_id IS NULL
        AND home_player_id IS NULL AND away_player_id IS NULL)
);
