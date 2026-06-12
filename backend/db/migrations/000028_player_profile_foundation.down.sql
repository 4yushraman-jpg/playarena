-- =============================================================================
-- Migration  : 000028_player_profile_foundation (DOWN)
-- Description: Reverses GP-1 identity foundation.
--
--              SAFETY: organization_id NOT NULL is restored ONLY when no global
--              (null-org) profiles exist. Once a global profile has been created
--              (organization_id IS NULL), restoring NOT NULL would fail / lose
--              data, so the column is deliberately left nullable with a notice.
--              This makes a full schema-down a one-way operation after global
--              profiles exist, consistent with the staged ownership-removal plan.
-- =============================================================================

DROP INDEX IF EXISTS uq_players_user_id;

ALTER TABLE players DROP CONSTRAINT IF EXISTS chk_players_visibility;
ALTER TABLE players DROP COLUMN IF EXISTS visibility;
ALTER TABLE players DROP COLUMN IF EXISTS archived_at;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM players WHERE organization_id IS NULL) THEN
        ALTER TABLE players ALTER COLUMN organization_id SET NOT NULL;
    ELSE
        RAISE NOTICE 'Global profiles exist (organization_id IS NULL); leaving organization_id nullable.';
    END IF;
END $$;
