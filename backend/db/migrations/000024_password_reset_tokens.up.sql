-- =============================================================================
-- Migration  : 000024_password_reset_tokens (UP)
-- Description: Introduces the password_reset_tokens table for the secure
--              password reset flow. Tokens are stored as SHA-256 hashes;
--              the raw token is sent to the user via email and never
--              persisted. Each token is single-use (used_at IS NULL = valid)
--              and expires after 1 hour (expires_at).
--
--              Consumption is done inside a FOR UPDATE transaction to prevent
--              concurrent double-use (same pattern as email_verification_tokens).
--
-- Depends on : 000003 (users table)
-- =============================================================================

CREATE TABLE password_reset_tokens (
    id          UUID        NOT NULL DEFAULT gen_random_uuid(),
    user_id     UUID        NOT NULL,
    token_hash  TEXT        NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT pk_password_reset_tokens          PRIMARY KEY (id),
    CONSTRAINT uq_password_reset_tokens_hash     UNIQUE (token_hash),
    CONSTRAINT fk_password_reset_tokens_user     FOREIGN KEY (user_id)
        REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT chk_password_reset_tokens_expires CHECK (expires_at > created_at)
);

COMMENT ON TABLE password_reset_tokens IS
    'Single-use password reset tokens. Stores only the SHA-256 hash of the raw '
    'token — the raw token is emailed to the user and never stored here. '
    'A token is valid when: used_at IS NULL AND expires_at > NOW(). '
    'Expired and used rows are removed by the background cleanup scheduler.';

COMMENT ON COLUMN password_reset_tokens.token_hash IS
    'SHA-256 hex-encoded hash of the raw reset token. '
    'Lookup: WHERE token_hash = encode(digest($raw, ''sha256''), ''hex'').';

COMMENT ON COLUMN password_reset_tokens.used_at IS
    'Set to NOW() when the token is consumed by a successful password reset. '
    'Also set on all sibling tokens for the same user when any reset completes, '
    'preventing stale tokens from being used after the password has changed.';

-- Find all valid tokens for a user (used for invalidation on reset completion).
CREATE INDEX idx_password_reset_tokens_user_id ON password_reset_tokens (user_id);

-- Cleanup job: "find all unused tokens that have expired".
CREATE INDEX idx_password_reset_tokens_expires ON password_reset_tokens (expires_at)
    WHERE used_at IS NULL;
