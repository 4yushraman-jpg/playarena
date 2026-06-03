-- Password reset token queries

-- name: CreatePasswordResetToken :one
INSERT INTO password_reset_tokens (user_id, token_hash, expires_at)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetPasswordResetTokenByHashForUpdate :one
-- Acquires a row-level exclusive lock on the token row.
-- Used by ResetPasswordTransaction to serialize concurrent consumption:
-- the second concurrent call blocks until the first commits, then reads
-- used_at IS NOT NULL and returns ErrResetTokenUsed.
SELECT *
FROM   password_reset_tokens
WHERE  token_hash = $1
FOR UPDATE;

-- name: UsePasswordResetToken :exec
-- Marks a specific token as consumed. The AND used_at IS NULL guard prevents
-- a concurrent double-use from silently succeeding.
UPDATE password_reset_tokens
SET    used_at = NOW()
WHERE  id      = $1
  AND  used_at IS NULL;

-- name: UseAllUserPasswordResetTokens :exec
-- Invalidates all outstanding (unused) reset tokens for a user after a
-- successful reset. Prevents stale tokens issued before the reset from being
-- used to change the password again.
UPDATE password_reset_tokens
SET    used_at = NOW()
WHERE  user_id = $1
  AND  used_at IS NULL;

-- name: DeleteExpiredPasswordResetTokens :exec
-- Removes tokens past their expiry. Called by the background cleanup scheduler.
-- Both unused-expired and used tokens are eligible for deletion.
DELETE FROM password_reset_tokens
WHERE  expires_at <= $1;
