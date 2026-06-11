-- Tournaments queries
-- organization_id is always required to enforce tenant isolation.

-- name: GetTournamentByID :one
SELECT *
FROM   tournaments
WHERE  id              = $1
  AND  organization_id = $2
LIMIT  1;

-- name: GetTournamentBySlug :one
SELECT *
FROM   tournaments
WHERE  slug            = $1
  AND  organization_id = $2
LIMIT  1;

-- name: ListTournamentsByOrganization :many
SELECT *
FROM   tournaments
WHERE  organization_id = $1
ORDER  BY created_at DESC;

-- name: CreateTournament :one
-- Inserts a new tournament in draft status. settings defaults to '{}'.
INSERT INTO tournaments (
    organization_id, name, slug, description, sport, format, participant_type,
    banner_url, prize_pool, currency, max_participants, min_participants,
    registration_opens_at, registration_closes_at, starts_at, ends_at,
    venue, city, country, rules, created_by
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
RETURNING *;

-- name: UpdateTournament :one
-- Full field update. Service layer merges partial request fields over current
-- state. id, organization_id, slug, and created_by are immutable.
UPDATE tournaments
SET    name                   = $3,
       description            = $4,
       sport                  = $5,
       format                 = $6,
       participant_type       = $7,
       banner_url             = $8,
       prize_pool             = $9,
       currency               = $10,
       max_participants       = $11,
       min_participants       = $12,
       registration_opens_at  = $13,
       registration_closes_at = $14,
       starts_at              = $15,
       ends_at                = $16,
       venue                  = $17,
       city                   = $18,
       country                = $19,
       rules                  = $20,
       status                 = $21,
       updated_at             = NOW()
WHERE  id              = $1
  AND  organization_id = $2
RETURNING *;

-- name: CancelTournament :one
-- Soft-delete: sets status to 'cancelled'. Records are retained permanently
-- because future registrations and match history will reference them.
UPDATE tournaments
SET    status     = 'cancelled',
       updated_at = NOW()
WHERE  id              = $1
  AND  organization_id = $2
RETURNING *;

-- name: ListTournamentsPaginated :many
-- Returns non-cancelled tournaments for an org with optional status and name
-- search filters. Cancelled tournaments remain accessible via GetTournamentByID.
SELECT *
FROM   tournaments
WHERE  organization_id = sqlc.arg(organization_id)
  AND  status != 'cancelled'
  AND  (sqlc.narg(status_filter)::text IS NULL OR status::text = sqlc.narg(status_filter))
  AND  (sqlc.narg(search_query)::text  IS NULL OR name ILIKE '%' || sqlc.narg(search_query) || '%')
ORDER  BY created_at DESC
LIMIT  sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- name: CountTournamentsByOrganization :one
-- Returns the total row count matching the same filters as ListTournamentsPaginated.
SELECT COUNT(*)
FROM   tournaments
WHERE  organization_id = sqlc.arg(organization_id)
  AND  status != 'cancelled'
  AND  (sqlc.narg(status_filter)::text IS NULL OR status::text = sqlc.narg(status_filter))
  AND  (sqlc.narg(search_query)::text  IS NULL OR name ILIKE '%' || sqlc.narg(search_query) || '%');

-- name: CountRegistrationsByStatusForTournaments :many
-- Aggregated per-status registration counts for a set of tournaments.
-- Drives the registration_counts block embedded in tournament responses so
-- clients never have to page through registrations to compute totals.
SELECT tournament_id, status, COUNT(*) AS count
FROM   tournament_registrations
WHERE  tournament_id = ANY(sqlc.arg(tournament_ids)::uuid[])
GROUP  BY tournament_id, status;

-- name: ListTeamNamesByIDs :many
-- Batch name lookup used to label standings participants.
SELECT id, name
FROM   teams
WHERE  id = ANY(sqlc.arg(team_ids)::uuid[]);

-- name: ListPlayerNamesByIDs :many
-- Batch name lookup used to label standings participants.
SELECT id, display_name
FROM   players
WHERE  id = ANY(sqlc.arg(player_ids)::uuid[]);
