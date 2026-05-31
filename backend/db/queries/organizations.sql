-- Organizations queries
-- All writes go through the service layer; only reads are generated here.

-- name: GetOrganizationByID :one
SELECT *
FROM   organizations
WHERE  id = $1
LIMIT  1;

-- name: GetOrganizationBySlug :one
SELECT *
FROM   organizations
WHERE  slug = $1
LIMIT  1;

-- name: ListOrganizations :many
SELECT *
FROM   organizations
ORDER  BY created_at DESC;
