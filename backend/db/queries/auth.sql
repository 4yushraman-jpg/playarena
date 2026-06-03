-- Authentication queries
-- Refresh token lifecycle management

-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (user_id, token_hash, expires_at, ip_address, user_agent)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetRefreshTokenByHash :one
SELECT *
FROM   refresh_tokens
WHERE  token_hash = $1
LIMIT  1;

-- name: GetRefreshTokenByHashForUpdate :one
SELECT *
FROM   refresh_tokens
WHERE  token_hash = $1
FOR UPDATE;

-- name: RevokeRefreshToken :exec
UPDATE refresh_tokens
SET    revoked_at = NOW()
WHERE  id = $1
  AND  revoked_at IS NULL;

-- name: RevokeUserRefreshTokens :exec
UPDATE refresh_tokens
SET    revoked_at = NOW()
WHERE  user_id = $1
  AND  revoked_at IS NULL;

-- name: RevokeAndLinkSuccessor :execrows
-- Atomically revokes a token and records the ID of its successor.
-- Used exclusively during token rotation: the new token must be inserted first
-- so its ID is available as $2. Returns the number of rows affected; the caller
-- must assert exactly 1 (enforced by the FOR UPDATE lock held on the old token).
UPDATE refresh_tokens
SET    revoked_at   = NOW(),
       successor_id = $2
WHERE  id           = $1
  AND  revoked_at   IS NULL;

-- name: DeleteExpiredRefreshTokens :exec
DELETE FROM refresh_tokens
WHERE  expires_at <= $1;
