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
