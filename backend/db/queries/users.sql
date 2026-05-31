-- Users queries
-- Passwords are handled outside generated code; only safe read projections here.

-- name: GetUserByID :one
SELECT *
FROM   users
WHERE  id = $1
LIMIT  1;

-- name: GetUserByEmail :one
-- Used by the authentication flow for login.
SELECT *
FROM   users
WHERE  email = $1
LIMIT  1;

-- name: GetUserByUsername :one
SELECT *
FROM   users
WHERE  username = $1
LIMIT  1;

-- name: ListUsers :many
SELECT *
FROM   users
ORDER  BY created_at DESC;
