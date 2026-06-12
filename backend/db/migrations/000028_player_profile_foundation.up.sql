-- =============================================================================
-- Migration  : 000028_player_profile_foundation (UP)
-- Phase      : GP-1 — Global PlayerProfile foundation.
-- Description: Evolves `players` in place into the spine of a global,
--              user-owned PlayerProfile WITHOUT removing organization ownership
--              yet (CASCADE FK retained; removal is staged to a later phase).
--
--              1. Add columns (visibility, archived_at).
--              2. Relax organization_id NOT NULL → a global profile has no org.
--              3. Deduplicate: keep the earliest profile per user_id, archive
--                 the rest (no merge, no FK repoint — the no-claim policy).
--              4. Enforce 1:1 (One User → One PlayerProfile) via a partial
--                 unique index over canonical (non-archived) rows.
--
--              A dry-run dedup report SHOULD be run first; see
--              db/reports/000028_dedup_dryrun.sql.
-- Depends on : 000005 (players)
-- =============================================================================

-- 1) additive columns (safe, no data dependency)
ALTER TABLE players ADD COLUMN visibility  TEXT NOT NULL DEFAULT 'private';
ALTER TABLE players ADD COLUMN archived_at TIMESTAMPTZ;

ALTER TABLE players
    ADD CONSTRAINT chk_players_visibility
    CHECK (visibility IN ('public', 'unlisted', 'private'));

-- 2) relax org ownership (column still populated; CASCADE FK unchanged in GP-1)
ALTER TABLE players ALTER COLUMN organization_id DROP NOT NULL;

-- 3) dedup BEFORE the unique index: keep the earliest row per user_id, archive
--    the rest. Deterministic survivor: MIN(created_at), tiebreak MIN(id).
--    FK references are NOT repointed (no-claim policy) — archived rows retain
--    their history in place.
WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (
               PARTITION BY user_id
               ORDER BY created_at ASC, id ASC
           ) AS rn
    FROM   players
    WHERE  user_id IS NOT NULL
      AND  archived_at IS NULL
)
UPDATE players p
SET    archived_at = NOW(),
       updated_at  = NOW()
FROM   ranked r
WHERE  p.id = r.id
  AND  r.rn > 1;

-- 4) enforce 1:1 (canonical, non-archived rows only)
CREATE UNIQUE INDEX uq_players_user_id
    ON players (user_id)
    WHERE user_id IS NOT NULL AND archived_at IS NULL;

COMMENT ON COLUMN players.organization_id IS
    'Nullable from GP-1. NULL = global, user-owned profile with no origin org. '
    'Non-NULL = legacy org-created profile. Ownership semantics removed in a later phase.';
COMMENT ON COLUMN players.archived_at IS
    'Non-NULL marks a non-canonical duplicate identity (legacy multi-org rows for one user). '
    'Archived rows are read-only, non-ranking, retained for history. Never merged (no-claim policy).';
COMMENT ON COLUMN players.visibility IS
    'public | unlisted | private. Default private (privacy-safe). Consumed by global profile reads.';
