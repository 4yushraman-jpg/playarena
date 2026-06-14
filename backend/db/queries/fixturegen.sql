-- Fixture generation queries (FE-8C).
-- organization_id is always required to enforce tenant isolation.

-- name: LockTournamentForGeneration :one
-- Locks the tournament row FOR UPDATE for the duration of a generation /
-- resolve-qualifiers transaction, returning the fields the generator needs:
-- status (the registration_closed / ongoing guard), format, participant_type
-- (team vs individual column routing), and settings (for explainability merge).
SELECT id, status, format, participant_type, settings
FROM   tournaments
WHERE  id              = $1
  AND  organization_id = $2
FOR    UPDATE;

-- name: CountMatchesForTournament :one
-- Total match rows for a tournament. Generation requires this to be 0 (no
-- fixtures exist) so it can never run twice or over a partial set.
SELECT COUNT(*)
FROM   matches
WHERE  tournament_id   = $1
  AND  organization_id = $2;

-- name: InsertGeneratedMatch :one
-- Inserts one generated match. Unlike CreateMatch this carries metadata (for
-- group-qualifier slot mapping) and the bracket edge, and it does NOT check the
-- tournament status — the generation service enforces registration_closed under
-- the FOR UPDATE lock instead. Participants may be NULL (TBD/qualifier slots);
-- the relaxed chk_matches_participants (FE-8B) permits partial/empty fills.
INSERT INTO matches (
    tournament_id,
    organization_id,
    round_number,
    round_name,
    match_number,
    home_team_id,
    away_team_id,
    home_player_id,
    away_player_id,
    status,
    group_label,
    next_match_id,
    next_match_slot,
    metadata
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'scheduled', $10, $11, $12, $13)
RETURNING *;

-- name: UpdateTournamentSettings :exec
-- Persists the merged settings JSONB (including the generation audit object).
UPDATE tournaments
SET    settings   = $3,
       updated_at = NOW()
WHERE  id              = $1
  AND  organization_id = $2;

-- name: CountUnfinishedGroupMatches :one
-- Counts group-stage matches that have not yet reached a final result. Qualifier
-- resolution requires this to be 0 (the whole group stage is decided). Cancelled
-- group matches are treated as resolved (they yield no result either way).
SELECT COUNT(*)
FROM   matches
WHERE  tournament_id   = $1
  AND  organization_id = $2
  AND  group_label IS NOT NULL
  AND  status NOT IN ('completed', 'walkover', 'cancelled');

-- name: ListGroupMatchesForStandings :many
-- Returns concluded group-stage matches (per-group standings input). Mirrors the
-- standings feed but scoped to grouped matches and carrying the group_label so
-- the caller can partition by group.
SELECT group_label,
       home_team_id,
       away_team_id,
       home_player_id,
       away_player_id,
       winner_team_id,
       winner_player_id,
       is_walkover,
       home_score,
       away_score
FROM   matches
WHERE  tournament_id   = $1
  AND  organization_id = $2
  AND  group_label IS NOT NULL
  AND  status IN ('completed', 'walkover')
ORDER  BY group_label, created_at ASC;

-- name: ListKnockoutQualifierMatches :many
-- Returns the knockout matches that still carry unresolved qualifier slots
-- (metadata holds the group/rank mapping). Used by resolve-qualifiers to fill
-- round-one knockout slots from the computed group standings.
SELECT id, round_number, home_team_id, away_team_id, home_player_id, away_player_id, metadata
FROM   matches
WHERE  tournament_id   = $1
  AND  organization_id = $2
  AND  group_label IS NULL
  AND  metadata ? 'qualifiers'
ORDER  BY round_number, match_number;
