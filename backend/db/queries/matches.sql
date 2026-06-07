-- Matches queries
-- organization_id is always required to enforce tenant isolation.

-- name: GetMatchByID :one
SELECT *
FROM   matches
WHERE  id              = $1
  AND  organization_id = $2
LIMIT  1;

-- name: ListMatchesByTournament :many
SELECT *
FROM   matches
WHERE  tournament_id   = $1
  AND  organization_id = $2
ORDER  BY round_number ASC NULLS LAST,
          match_number  ASC NULLS LAST;

-- name: CreateMatch :one
-- Inserts a new scheduled match fixture. organization_id must equal the parent
-- tournament's organization_id; this is enforced by trg_matches_org_consistency.
-- status is always 'scheduled' on creation; is_walkover always FALSE.
-- metadata defaults to '{}' via the table default.
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
    venue,
    scheduled_at,
    status,
    notes
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING *;

-- name: UpdateMatch :one
-- Full mutable-field update with compare-and-swap (CAS) status guard.
-- Service layer merges partial request fields over current state.
-- id, organization_id, tournament_id, is_walkover, metadata, and created_at
-- are immutable and never appear in the SET clause.
-- started_at and ended_at are stamped by the service during lifecycle transitions.
-- home_score and away_score ($18, $19) are 0 for all non-completion transitions;
-- for the live → completed transition the repository fills them in after computing
-- the final score from the effective event log under a FOR UPDATE lock on this row.
-- CAS guard: AND status = $20 (previous_status from the service read) ensures
-- that if a concurrent request already changed the status between the service
-- read and this write, this UPDATE matches 0 rows and returns ErrNoRows, which
-- the repository maps to ErrMatchNotUpdatable.
UPDATE matches
SET    round_number     = $3,
       round_name       = $4,
       match_number     = $5,
       home_team_id     = $6,
       away_team_id     = $7,
       home_player_id   = $8,
       away_player_id   = $9,
       venue            = $10,
       scheduled_at     = $11,
       started_at       = $12,
       ended_at         = CASE WHEN $13::TIMESTAMPTZ IS NOT NULL THEN GREATEST($13::TIMESTAMPTZ, started_at + INTERVAL '1 millisecond') ELSE NULL END,
       status           = $14,
       winner_team_id   = $15,
       winner_player_id = $16,
       notes            = $17,
       home_score       = $18,
       away_score       = $19,
       updated_at       = NOW()
WHERE  id              = $1
  AND  organization_id = $2
  AND  status          = $20
RETURNING *;

-- name: CancelMatch :one
-- Soft-cancel: sets status to 'cancelled'. Records are never hard-deleted so
-- that future match_events and audit_log references remain resolvable.
-- CAS guard: AND status = $3 (previous_status) ensures a concurrent transition
-- that already moved the match to a terminal state causes this UPDATE to match
-- 0 rows, returning ErrNoRows → ErrMatchNotUpdatable in the repository.
UPDATE matches
SET    status     = 'cancelled',
       updated_at = NOW()
WHERE  id              = $1
  AND  organization_id = $2
  AND  status          = $3
RETURNING *;

-- name: ListMatchesPaginated :many
-- Paginated listing of matches for an org.
-- Optional tournament_id, status, and text search (venue / round_name) filters.
SELECT *
FROM   matches
WHERE  organization_id = sqlc.arg(organization_id)
  AND  (sqlc.narg(tournament_id_filter)::uuid IS NULL
        OR tournament_id = sqlc.narg(tournament_id_filter)::uuid)
  AND  (sqlc.narg(status_filter)::text IS NULL
        OR status::text = sqlc.narg(status_filter))
  AND  (sqlc.narg(search_query)::text IS NULL
        OR venue      ILIKE '%' || sqlc.narg(search_query) || '%'
        OR round_name ILIKE '%' || sqlc.narg(search_query) || '%')
ORDER  BY scheduled_at ASC NULLS LAST,
          created_at   DESC
LIMIT  sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- name: CountMatches :one
-- Returns the total count matching the same filters as ListMatchesPaginated.
-- Used to build pagination metadata without fetching rows.
SELECT COUNT(*)
FROM   matches
WHERE  organization_id = sqlc.arg(organization_id)
  AND  (sqlc.narg(tournament_id_filter)::uuid IS NULL
        OR tournament_id = sqlc.narg(tournament_id_filter)::uuid)
  AND  (sqlc.narg(status_filter)::text IS NULL
        OR status::text = sqlc.narg(status_filter))
  AND  (sqlc.narg(search_query)::text IS NULL
        OR venue      ILIKE '%' || sqlc.narg(search_query) || '%'
        OR round_name ILIKE '%' || sqlc.narg(search_query) || '%');

-- name: LockTournamentForShare :one
-- Acquires a row-level share lock on the tournament row inside a transaction.
-- Used during match create/update to prevent a concurrent tournament cancellation
-- from racing with a status-sensitive match operation.
-- FOR SHARE: blocks concurrent UPDATEs (cancellation) while allowing other
-- readers; does not block concurrent match creates for the same tournament.
SELECT status
FROM   tournaments
WHERE  id              = $1
  AND  organization_id = $2
FOR    SHARE;

-- name: ListCompletedMatchesByTournament :many
-- Returns all completed matches for a tournament, in creation order.
-- Used exclusively by the standings engine — standing computation MUST NOT
-- read match_events; it reads only these pre-snapshotted score columns.
-- Both organization_id and tournament_id are required to enforce multi-tenant
-- isolation: a caller can only read matches belonging to their own tournament.
SELECT id,
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
WHERE  tournament_id   = sqlc.arg(tournament_id)
  AND  organization_id = sqlc.arg(organization_id)
  AND  status          = 'completed'
ORDER  BY created_at ASC;
