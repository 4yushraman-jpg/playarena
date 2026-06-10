-- Tournament registration queries
-- tournament_id always required; organization_id is the registrant's org.

-- name: GetRegistrationByID :one
-- Fetches a single registration scoped to its tournament.
SELECT *
FROM   tournament_registrations
WHERE  id            = $1
  AND  tournament_id = $2
LIMIT  1;

-- name: GetRegistrationByTeam :one
-- Returns an existing registration for the given (tournament, team) pair,
-- regardless of status. Used to enforce Rule 4: no duplicate registrations.
SELECT *
FROM   tournament_registrations
WHERE  tournament_id = $1
  AND  team_id       = $2
LIMIT  1;

-- name: ListRegistrationsByTournamentPaginated :many
-- Paginated listing of all registrations for a tournament.
-- Optional status filter; no status is excluded by default.
SELECT *
FROM   tournament_registrations
WHERE  tournament_id = sqlc.arg(tournament_id)
  AND  (sqlc.narg(status_filter)::text IS NULL OR status::text = sqlc.narg(status_filter))
ORDER  BY registered_at ASC
LIMIT  sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- name: CountRegistrationsByTournament :one
-- Total count matching the same filters as ListRegistrationsByTournamentPaginated.
SELECT COUNT(*)
FROM   tournament_registrations
WHERE  tournament_id = sqlc.arg(tournament_id)
  AND  (sqlc.narg(status_filter)::text IS NULL OR status::text = sqlc.narg(status_filter));

-- name: CountActiveRegistrations :one
-- Counts pending + approved registrations against max_participants capacity.
-- Withdrawn, rejected, and disqualified registrations do not count.
SELECT COUNT(*)
FROM   tournament_registrations
WHERE  tournament_id = $1
  AND  status        IN ('pending', 'approved');

-- name: LockTournamentForUpdate :one
-- Acquires an exclusive row-level lock on the tournament row inside a
-- transaction. All concurrent registrations for the same tournament block
-- here until the holding transaction commits or rolls back, preventing
-- concurrent reads of the same capacity count from racing to insert.
SELECT id
FROM   tournaments
WHERE  id = $1
FOR    UPDATE;

-- name: CreateRegistration :one
-- Inserts a new pending registration. The partial unique index
-- uq_treg_tournament_team prevents duplicate (tournament, team) pairs.
INSERT INTO tournament_registrations (
    tournament_id, organization_id, team_id, player_id,
    registered_by, notes
)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: UpdateRegistration :one
-- Updates mutable fields. Service layer applies partial request over
-- current state. approved_by and approved_at are set on status=approved.
UPDATE tournament_registrations
SET    status      = $3,
       seed_number = $4,
       notes       = $5,
       approved_by = $6,
       approved_at = CASE WHEN $7::TIMESTAMPTZ IS NOT NULL THEN GREATEST($7::TIMESTAMPTZ, registered_at) ELSE NULL END,
       updated_at  = NOW()
WHERE  id            = $1
  AND  tournament_id = $2
RETURNING *;

-- name: GetApprovedRegistrationByTeam :one
-- Used by the matches module to verify a team has an approved registration
-- before being assigned as a match participant.
SELECT id
FROM   tournament_registrations
WHERE  tournament_id = $1
  AND  team_id       = $2
  AND  status        = 'approved'
LIMIT  1;

-- name: GetRegistrationByPlayer :one
-- Returns an existing registration for the given (tournament, player) pair,
-- regardless of status. Used to enforce Rule 4: no duplicate registrations.
SELECT *
FROM   tournament_registrations
WHERE  tournament_id = $1
  AND  player_id     = $2
LIMIT  1;

-- name: GetApprovedRegistrationByPlayer :one
-- Used by the matches module to verify a player has an approved registration
-- before being assigned as a match participant.
SELECT id
FROM   tournament_registrations
WHERE  tournament_id = $1
  AND  player_id     = $2
  AND  status        = 'approved'
LIMIT  1;

-- name: WithdrawRegistration :one
-- Soft-removes by setting status = 'withdrawn'.
-- Only acts on pending or approved registrations; returns no rows if already
-- in a terminal state (rejected, withdrawn, disqualified).
UPDATE tournament_registrations
SET    status     = 'withdrawn',
       updated_at = NOW()
WHERE  id            = $1
  AND  tournament_id = $2
  AND  status        IN ('pending', 'approved')
RETURNING *;

-- name: ListApprovedRegistrationsForStandings :many
-- Returns all approved registrations for a tournament for standings computation.
-- Does NOT filter by organization_id: tournament_registrations.organization_id
-- is the registrant's org (not the host org), and cross-org registrations are
-- valid for multi-club tournaments.  The host tournament is already verified by
-- the caller via tournament_id + host organization_id before this query runs.
-- Ordered by registered_at ASC so the standings engine uses registration time
-- as the final deterministic tiebreaker without additional sorting.
SELECT team_id, player_id, seed_number, registered_at
FROM   tournament_registrations
WHERE  tournament_id = $1
  AND  status        = 'approved'
ORDER  BY registered_at ASC;
