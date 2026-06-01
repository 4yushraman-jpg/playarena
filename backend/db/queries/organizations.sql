-- Organizations queries

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

-- name: ListOrganizationsPaginated :many
-- Paginated listing. limit is capped by the application layer (default 50, max 200).
-- offset is zero-based. Use created_at DESC for stable ordering across pages.
SELECT *
FROM   organizations
ORDER  BY created_at DESC
LIMIT  sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- name: CreateOrganization :one
INSERT INTO organizations (name, slug, description, type, website, email, phone, country, city)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: UpdateOrganization :one
-- All updatable fields are passed explicitly; the service layer applies the
-- caller's partial-update fields over the current org state before calling
-- this query.
-- slug and id are not updatable. status is managed via dedicated transitions.
UPDATE organizations
SET    name        = $2,
       description = $3,
       type        = $4,
       website     = $5,
       email       = $6,
       phone       = $7,
       country     = $8,
       city        = $9,
       updated_at  = NOW()
WHERE  id = $1
RETURNING *;

-- name: DeleteOrganization :exec
-- Hard delete. Cascades to all child records (teams, players, tournaments, etc.)
-- via the FK ON DELETE CASCADE chain. Caller must verify authorization before
-- invoking.
DELETE FROM organizations
WHERE  id = $1;
