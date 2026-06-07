-- Players queries
-- organization_id is always required to enforce tenant isolation.

-- name: GetPlayerByID :one
SELECT *
FROM   players
WHERE  id              = $1
  AND  organization_id = $2
LIMIT  1;

-- name: ListPlayersByOrganization :many
SELECT *
FROM   players
WHERE  organization_id = $1
ORDER  BY display_name ASC;

-- name: CreatePlayer :one
-- Inserts a new active player profile for the given organization.
-- status defaults to 'active', metadata to '{}' per table defaults.
INSERT INTO players (
    organization_id,
    user_id,
    display_name,
    jersey_number,
    position,
    height_cm,
    weight_kg,
    dominant_hand,
    nationality,
    date_of_birth,
    bio
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: UpdatePlayer :one
-- Full field update for an existing player. Service layer applies partial
-- request fields over current state before calling this query.
-- id and organization_id are immutable and used only in the WHERE clause.
UPDATE players
SET    display_name  = $3,
       jersey_number = $4,
       position      = $5,
       height_cm     = $6,
       weight_kg     = $7,
       dominant_hand = $8,
       nationality   = $9,
       date_of_birth = $10,
       bio           = $11,
       status        = $12,
       updated_at    = NOW()
WHERE  id              = $1
  AND  organization_id = $2
RETURNING *;

-- name: SoftDeletePlayer :one
-- Marks a player as inactive (soft delete). Records are never hard-deleted
-- so that historical team membership and match event data remains intact.
UPDATE players
SET    status     = 'inactive',
       updated_at = NOW()
WHERE  id              = $1
  AND  organization_id = $2
RETURNING *;

-- name: ListPlayersPaginated :many
-- Returns players for an org with optional status and name search filters.
-- When no status_filter is given, inactive (soft-deleted) players are excluded.
-- When status_filter is provided, it overrides the default exclusion so callers
-- can explicitly request inactive players.
SELECT *
FROM   players
WHERE  organization_id = sqlc.arg(organization_id)
  AND  (sqlc.narg(status_filter)::text IS NOT NULL OR status != 'inactive')
  AND  (sqlc.narg(status_filter)::text IS NULL     OR status::text = sqlc.narg(status_filter))
  AND  (sqlc.narg(search_query)::text  IS NULL OR display_name ILIKE '%' || sqlc.narg(search_query) || '%')
ORDER  BY display_name ASC
LIMIT  sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- name: CountPlayersByOrganization :one
-- Returns the total row count matching the same filters as ListPlayersPaginated.
-- Used to build pagination metadata without fetching rows.
SELECT COUNT(*)
FROM   players
WHERE  organization_id = sqlc.arg(organization_id)
  AND  (sqlc.narg(status_filter)::text IS NOT NULL OR status != 'inactive')
  AND  (sqlc.narg(status_filter)::text IS NULL     OR status::text = sqlc.narg(status_filter))
  AND  (sqlc.narg(search_query)::text  IS NULL OR display_name ILIKE '%' || sqlc.narg(search_query) || '%');
