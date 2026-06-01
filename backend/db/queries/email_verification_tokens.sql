-- Email verification token queries

-- name: CreateEmailVerificationToken :one
INSERT INTO email_verification_tokens (user_id, token_hash, expires_at)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetEmailVerificationTokenByHash :one
SELECT *
FROM   email_verification_tokens
WHERE  token_hash = $1
LIMIT  1;

-- name: UseEmailVerificationToken :exec
-- Marks a token as consumed. The AND used_at IS NULL guard prevents a
-- double-use race condition from silently succeeding.
UPDATE email_verification_tokens
SET    used_at = NOW()
WHERE  id      = $1
  AND  used_at IS NULL;

-- name: DeleteExpiredEmailVerificationTokens :exec
-- Removes tokens that have passed their expiry. Called by the background
-- cleanup job. Unused expired tokens are safe to delete: they can never
-- be consumed again.
DELETE FROM email_verification_tokens
WHERE  expires_at <= $1;
