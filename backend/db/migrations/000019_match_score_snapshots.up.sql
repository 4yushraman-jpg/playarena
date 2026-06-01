-- =============================================================================
-- Migration  : 000019_match_score_snapshots (UP)
-- Description: Adds home_score and away_score columns to the matches table.
--
--              These columns are the Phase 10 score snapshot — the bridge
--              between the event-sourcing scoring layer (match_events) and the
--              standings computation layer.
--
--              IMMUTABILITY CONTRACT:
--              Both columns are written EXACTLY ONCE per match, atomically
--              inside the live → completed transition transaction, under a
--              FOR UPDATE lock on the match row.  At that moment the effective
--              event log is permanently frozen (no new events can be added to
--              a non-live match), so the snapshot is permanently valid.
--
--              STANDINGS RULE:
--              Standings computation reads home_score / away_score from
--              completed matches only.  It NEVER reads match_events.
--              match_events remains the sole source of truth for live scores.
--
-- Depends on : 000010 (matches)
-- =============================================================================

ALTER TABLE matches
    ADD COLUMN home_score INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN away_score INTEGER NOT NULL DEFAULT 0;

COMMENT ON COLUMN matches.home_score IS
    'Final home participant score. Snapshotted atomically inside the '
    'live → completed transition transaction under a FOR UPDATE lock. '
    'Reflects the effective event log at the exact moment of completion — '
    'the log is frozen by the match-status check in CreateWithAudit, so '
    'no correction can alter this value after it is written. '
    'Always 0 for walkovers (no events produce points) and for any '
    'non-completed match status. Standings must read this column, never '
    'match_events, for completed match scores.';

COMMENT ON COLUMN matches.away_score IS
    'Final away participant score. See home_score for the full contract.';
