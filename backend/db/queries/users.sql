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

-- name: CreateUser :one
-- Inserts a new platform user. status defaults to pending_verification;
-- all nullable profile columns are left NULL until the user fills them in.
INSERT INTO users (email, username, password_hash, first_name, last_name)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: VerifyUserEmail :exec
-- Activates the account after the user clicks the verification link.
-- The AND status = 'pending_verification' guard makes the call idempotent:
-- if the account is already active (e.g. concurrent verification), it is a
-- safe no-op.
UPDATE users
SET    email_verified_at = NOW(),
       status            = 'active',
       updated_at        = NOW()
WHERE  id     = $1
  AND  status = 'pending_verification';
