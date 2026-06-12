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
  AND  archived_at IS NULL
ORDER  BY display_name ASC;

-- ── GP-1: global PlayerProfile (user-owned) ────────────────────────────────

-- name: GetPlayerProfileByUserID :one
-- The caller's canonical (non-archived) profile, regardless of org.
-- One User -> One PlayerProfile is enforced by uq_players_user_id.
SELECT *
FROM   players
WHERE  user_id = $1
  AND  archived_at IS NULL
LIMIT  1;

-- name: GetPlayerProfileByID :one
-- A single profile by id, excluding archived duplicates. Used by the global
-- read endpoint (visibility filtering is applied in the service layer).
SELECT *
FROM   players
WHERE  id = $1
  AND  archived_at IS NULL
LIMIT  1;

-- name: CreateGlobalPlayerProfile :one
-- Owner-created profile: organization_id is NULL (global, user-owned).
INSERT INTO players (
    user_id,
    display_name,
    jersey_number,
    position,
    height_cm,
    weight_kg,
    dominant_hand,
    nationality,
    date_of_birth,
    bio,
    visibility
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: UpdateOwnPlayerProfile :one
-- Identity-field update by the owner. organization_id / user_id / status are
-- intentionally NOT mutable here. The WHERE clause binds the row to its owner.
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
       visibility    = $12,
       updated_at    = NOW()
WHERE  id      = $1
  AND  user_id = $2
  AND  archived_at IS NULL
RETURNING *;

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
  AND  archived_at IS NULL
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
  AND  archived_at IS NULL
  AND  (sqlc.narg(status_filter)::text IS NOT NULL OR status != 'inactive')
  AND  (sqlc.narg(status_filter)::text IS NULL     OR status::text = sqlc.narg(status_filter))
  AND  (sqlc.narg(search_query)::text  IS NULL OR display_name ILIKE '%' || sqlc.narg(search_query) || '%');
