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

-- name: UpdateUserPasswordHash :exec
-- Replaces the stored bcrypt hash. Called exclusively by ResetPasswordTransaction
-- inside a FOR UPDATE token-locked transaction so the password update is atomic
-- with token consumption and session revocation.
UPDATE users
SET    password_hash = $1,
       updated_at    = NOW()
WHERE  id = $2;

-- name: GetUserByIDForUpdate :one
-- Acquires a FOR UPDATE row lock on the user. Used inside transactions that must
-- prevent concurrent status changes or password updates (e.g. ChangePassword,
-- DeactivateUser). Always call inside an open transaction.
SELECT *
FROM   users
WHERE  id = $1
FOR UPDATE;

-- name: UpdateUserProfile :one
-- Replaces all editable profile fields in a single statement. The service layer
-- reads the current row first and merges the PATCH fields, so every column is
-- always explicitly supplied — no COALESCE needed here.
-- email, password_hash, status, and verification fields are intentionally absent.
UPDATE users
SET    first_name    = $2,
       last_name     = $3,
       username      = $4,
       phone         = $5,
       date_of_birth = $6,
       gender        = $7,
       updated_at    = NOW()
WHERE  id = $1
RETURNING *;

-- name: ListUsersPaginated :many
-- Returns a page of users ordered by creation time (newest first).
-- Limit and offset are validated and capped by the service layer before reaching here.
SELECT *
FROM   users
ORDER  BY created_at DESC
LIMIT  sqlc.arg(limit_count)
OFFSET sqlc.arg(offset_count);

-- name: CountUsers :one
SELECT COUNT(*) FROM users;

-- name: DeactivateUser :execrows
-- Sets user status to inactive. Returns the number of rows affected:
--   1 → user was active/suspended and has been deactivated.
--   0 → user was already inactive (idempotent guard).
-- The caller checks the count and returns ErrUserAlreadyInactive when 0.
UPDATE users
SET    status     = 'inactive',
       updated_at = NOW()
WHERE  id         = $1
  AND  status    != 'inactive';

-- name: IsUserActivePlatformAdmin :one
-- Returns true when the user holds a non-expired platform_admin grant.
-- Used inside DeactivateTransaction to decide whether the last-admin guard
-- needs to fire. Runs before the sentinel lock is acquired.
SELECT EXISTS(
    SELECT 1
    FROM   user_organization_roles uor
    JOIN   roles r ON r.id = uor.role_id
    WHERE  uor.user_id         = $1
      AND  r.slug              = 'platform_admin'
      AND  r.scope             = 'platform'
      AND  uor.organization_id IS NULL
      AND  (uor.expires_at IS NULL OR uor.expires_at > NOW())
) AS is_platform_admin;

-- name: GetPlatformAdminRoleForUpdate :one
-- Acquires a FOR UPDATE lock on the platform_admin role row. All concurrent
-- DeactivateUser transactions that target a platform admin must acquire this
-- lock before counting — serialising the count-then-deactivate check and
-- closing the TOCTOU window where two concurrent deactivations could each
-- read a count of 2 and both proceed, leaving zero admins.
SELECT id
FROM   roles
WHERE  slug              = 'platform_admin'
  AND  scope             = 'platform'
  AND  organization_id IS NULL
LIMIT  1
FOR UPDATE;

-- name: CountActivePlatformAdmins :one
-- Counts distinct users who currently hold an active platform_admin grant
-- and whose account status is 'active'. Called inside DeactivateTransaction
-- after the sentinel lock is held, so the count reflects committed state.
SELECT COUNT(DISTINCT u.id)
FROM   users u
JOIN   user_organization_roles uor ON uor.user_id = u.id
JOIN   roles r ON r.id = uor.role_id
WHERE  r.slug              = 'platform_admin'
  AND  r.scope             = 'platform'
  AND  uor.organization_id IS NULL
  AND  u.status            = 'active'
  AND  (uor.expires_at IS NULL OR uor.expires_at > NOW());
