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
