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
