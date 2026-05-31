-- =============================================================================
-- Migration  : 000003_create_users (UP)
-- Description: Creates the users table (platform-level identity) and the
--              refresh_tokens table (JWT revocation store).
--              refresh_tokens is included here because it is tightly coupled
--              to users and has no other dependencies.
-- Depends on : 000001 (pgcrypto, user_status, gender ENUMs)
-- Note       : users has no organization_id — org membership is managed via
--              user_organization_roles (migration 000004).
-- =============================================================================

-- ---------------------------------------------------------------------------
-- users
-- ---------------------------------------------------------------------------

CREATE TABLE users (
    id                UUID        NOT NULL DEFAULT gen_random_uuid(),
    email             TEXT        NOT NULL,
    username          TEXT        NOT NULL,
    password_hash     TEXT        NOT NULL,
    first_name        TEXT        NOT NULL,
    last_name         TEXT        NOT NULL,
    phone             TEXT,
    avatar_url        TEXT,
    date_of_birth     DATE,
    gender            gender,
    status            user_status NOT NULL DEFAULT 'pending_verification',
    email_verified_at TIMESTAMPTZ,
    last_login_at     TIMESTAMPTZ,
    last_login_ip     INET,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT pk_users                PRIMARY KEY (id),
    CONSTRAINT uq_users_email          UNIQUE (email),
    CONSTRAINT uq_users_username       UNIQUE (username),
    CONSTRAINT chk_users_email         CHECK (email ~ '^[^@\s]+@[^@\s]+\.[^@\s]+$'),
    CONSTRAINT chk_users_username      CHECK (username ~ '^[a-zA-Z0-9_]{3,30}$'),
    CONSTRAINT chk_users_first_name    CHECK (char_length(trim(first_name)) >= 1),
    CONSTRAINT chk_users_last_name     CHECK (char_length(trim(last_name)) >= 1)
);

COMMENT ON TABLE users IS
    'Platform-level identity. One account per human being. '
    'Users are not directly scoped to an organization: membership is managed '
    'via the user_organization_roles join table. '
    'Super-admin users have platform-scoped roles with organization_id = NULL.';

COMMENT ON COLUMN users.email IS
    'Globally unique email address. Primary login identifier. '
    'Must be verified before status transitions to active.';

COMMENT ON COLUMN users.username IS
    'Globally unique display handle. 3–30 chars, alphanumeric and underscores only. '
    'Used in public profiles and @mentions.';

COMMENT ON COLUMN users.password_hash IS
    'bcrypt hash of the user password. Never store plaintext. '
    'Minimum bcrypt cost factor: 12.';

COMMENT ON COLUMN users.last_login_ip IS
    'INET type stores both IPv4 and IPv6 addresses natively. '
    'Used for security auditing and anomaly detection (e.g., new-country login).';

COMMENT ON COLUMN users.gender IS
    'Self-reported, optional. NULL means not provided — distinct from prefer_not_to_say '
    'which means the user explicitly declined.';

-- Status is queried frequently (auth middleware filters suspended/inactive users)
CREATE INDEX idx_users_status ON users (status);


-- ---------------------------------------------------------------------------
-- refresh_tokens
-- Included in this migration because it is a direct child of users with no
-- other foreign key dependencies.
-- ---------------------------------------------------------------------------

CREATE TABLE refresh_tokens (
    id          UUID        NOT NULL DEFAULT gen_random_uuid(),
    user_id     UUID        NOT NULL,
    token_hash  TEXT        NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked_at  TIMESTAMPTZ,
    ip_address  INET,
    user_agent  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- No updated_at: refresh_tokens are immutable after creation.
    -- Revocation is expressed by setting revoked_at, not by updating other fields.

    CONSTRAINT pk_refresh_tokens            PRIMARY KEY (id),
    CONSTRAINT uq_refresh_tokens_hash       UNIQUE (token_hash),
    CONSTRAINT fk_refresh_tokens_user       FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT chk_refresh_tokens_expires   CHECK (expires_at > created_at)
);

COMMENT ON TABLE refresh_tokens IS
    'JWT refresh token revocation store. '
    'Stores a hash of the raw token — the raw token is never persisted. '
    'A token is valid when: revoked_at IS NULL AND expires_at > NOW(). '
    'Expired and revoked rows are cleaned up by a periodic background job.';

COMMENT ON COLUMN refresh_tokens.token_hash IS
    'SHA-256 hash of the raw refresh token. '
    'Lookup: WHERE token_hash = encode(digest($raw_token, ''sha256''), ''hex'').';

COMMENT ON COLUMN refresh_tokens.revoked_at IS
    'Set to NOW() when the token is explicitly invalidated (logout, password change). '
    'NULL means the token has not been revoked yet.';

-- Find all valid tokens for a user (used on logout-all / password change)
CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens (user_id);

-- Expiry cleanup job: "find all non-revoked tokens that have expired"
CREATE INDEX idx_refresh_tokens_expires ON refresh_tokens (expires_at)
    WHERE revoked_at IS NULL;
