-- =============================================================================
-- Migration  : 000016_email_verification_tokens (UP)
-- Description: Creates the email_verification_tokens table used by the
--              user registration flow. A short-lived, single-use token is
--              generated on registration, hashed, and stored here. The raw
--              token is sent to the user's email address (or returned in the
--              registration response during development). Presenting the raw
--              token to GET /api/v1/auth/verify-email activates the account.
-- Depends on : 000003 (users)
-- =============================================================================

CREATE TABLE email_verification_tokens (
    id         UUID        NOT NULL DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL,
    token_hash TEXT        NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    -- NULL until the token is consumed. Set to NOW() on first use; subsequent
    -- attempts with the same token are rejected as already-used.
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT pk_email_verification_tokens       PRIMARY KEY (id),
    CONSTRAINT uq_email_verification_tokens_hash  UNIQUE (token_hash),
    CONSTRAINT fk_evt_user                        FOREIGN KEY (user_id)
                                                  REFERENCES users (id)
                                                  ON DELETE CASCADE,
    CONSTRAINT chk_evt_expires                    CHECK (expires_at > created_at)
);

COMMENT ON TABLE email_verification_tokens IS
    'Single-use email verification tokens. '
    'Stores a SHA-256 hash of the raw token — the raw value is never persisted. '
    'A token is valid when: used_at IS NULL AND expires_at > NOW(). '
    'Expired and used rows are cleaned up by a periodic background job.';

COMMENT ON COLUMN email_verification_tokens.token_hash IS
    'SHA-256 hex-encoded hash of the raw token sent to the user. '
    'Lookup: WHERE token_hash = encode(digest($raw_token, ''sha256''), ''hex'').';

COMMENT ON COLUMN email_verification_tokens.used_at IS
    'Set to NOW() the first time the token is successfully consumed. '
    'NULL means the token is still valid (pending use).';

-- Lookup by user: find all pending tokens for a user (e.g. resend flow)
CREATE INDEX idx_evt_user_id
    ON email_verification_tokens (user_id);

-- Cleanup job: "find all tokens that have expired and have not been used"
CREATE INDEX idx_evt_expires
    ON email_verification_tokens (expires_at)
    WHERE used_at IS NULL;
