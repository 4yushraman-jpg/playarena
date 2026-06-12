-- =============================================================================
-- GP-1 dedup DRY-RUN report (read-only). Run BEFORE applying migration 000028.
--
-- Lists every user_id that has more than one non-archived player row. These are
-- the rows the migration will deduplicate: the earliest (MIN(created_at), then
-- MIN(id)) is kept as the canonical profile; the rest are archived (archived_at
-- set). No rows are deleted; no foreign keys are repointed.
--
-- An empty result set means the migration archives nothing.
-- =============================================================================

SELECT user_id,
       COUNT(*)                         AS profile_count,
       MIN(created_at)                  AS earliest_created_at,
       array_agg(id ORDER BY created_at ASC, id ASC) AS profile_ids_kept_first
FROM   players
WHERE  user_id IS NOT NULL
  AND  archived_at IS NULL
GROUP  BY user_id
HAVING COUNT(*) > 1
ORDER  BY profile_count DESC, user_id;
