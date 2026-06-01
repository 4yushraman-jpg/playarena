-- Teams queries
-- organization_id is always required to enforce tenant isolation.

-- name: GetTeamByID :one
SELECT *
FROM   teams
WHERE  id              = $1
  AND  organization_id = $2
LIMIT  1;

-- name: GetTeamBySlug :one
SELECT *
FROM   teams
WHERE  slug            = $1
  AND  organization_id = $2
LIMIT  1;

-- name: ListTeamsByOrganization :many
SELECT *
FROM   teams
WHERE  organization_id = $1
ORDER  BY name ASC;

-- name: CreateTeam :one
-- Inserts a new active team for the given organization.
-- status defaults to 'active', metadata to '{}' per table defaults.
INSERT INTO teams (
    organization_id, name, short_name, slug, description,
    logo_url, home_city, home_venue, founded_year,
    primary_color, secondary_color
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: UpdateTeam :one
-- Full field update. Service layer merges partial request fields over current
-- state before calling this query. id, organization_id, slug are immutable.
UPDATE teams
SET    name            = $3,
       short_name      = $4,
       description     = $5,
       logo_url        = $6,
       home_city       = $7,
       home_venue      = $8,
       founded_year    = $9,
       primary_color   = $10,
       secondary_color = $11,
       status          = $12,
       updated_at      = NOW()
WHERE  id              = $1
  AND  organization_id = $2
RETURNING *;

-- name: DisbandTeam :one
-- Soft-delete: sets status to 'disbanded'. Records are retained permanently
-- because match winner references and ranking history depend on them.
UPDATE teams
SET    status     = 'disbanded',
       updated_at = NOW()
WHERE  id              = $1
  AND  organization_id = $2
RETURNING *;

-- name: ListTeamsPaginated :many
-- Returns non-disbanded teams for an org with optional status and name search.
-- Disbanded teams are excluded from default listings to mirror soft-delete
-- semantics; they remain accessible via GetTeamByID for historical resolution.
SELECT *
FROM   teams
WHERE  organization_id = sqlc.arg(organization_id)
  AND  status != 'disbanded'
  AND  (sqlc.narg(status_filter)::text IS NULL OR status::text = sqlc.narg(status_filter))
  AND  (sqlc.narg(search_query)::text  IS NULL OR name ILIKE '%' || sqlc.narg(search_query) || '%')
ORDER  BY name ASC
LIMIT  sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- name: CountTeamsByOrganization :one
-- Returns the total count matching the same filters as ListTeamsPaginated.
SELECT COUNT(*)
FROM   teams
WHERE  organization_id = sqlc.arg(organization_id)
  AND  status != 'disbanded'
  AND  (sqlc.narg(status_filter)::text IS NULL OR status::text = sqlc.narg(status_filter))
  AND  (sqlc.narg(search_query)::text  IS NULL OR name ILIKE '%' || sqlc.narg(search_query) || '%');
