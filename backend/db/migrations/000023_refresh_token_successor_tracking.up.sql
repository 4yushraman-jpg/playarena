-- =============================================================================
-- Migration  : 000023_refresh_token_successor_tracking (UP)
-- Description: Adds successor_id to refresh_tokens to enable deterministic
--              structural replay detection without time-based grace windows.
--
--              When a token is rotated the old token records the new token's ID
--              in successor_id. Replay detection reads this field:
--
--                successor_id IS NULL     → explicitly revoked (logout / admin)
--                successor_id IS NOT NULL → already rotated (race or replay)
--
--              successor_id is intentionally NOT a foreign key. It is a
--              historical state marker. The application never follows the
--              reference — it only checks IS NULL / IS NOT NULL. No ON DELETE
--              behaviour is needed or wanted: if the successor row is cleaned up
--              by the expiry job the classification (non-NULL) is preserved,
--              which is correct.
--
-- Depends on : 000003 (refresh_tokens table)
-- =============================================================================

ALTER TABLE refresh_tokens
    ADD COLUMN successor_id UUID NULL;

ALTER TABLE refresh_tokens
    ADD CONSTRAINT chk_refresh_tokens_successor
        CHECK (
            successor_id IS NULL
            OR revoked_at IS NOT NULL
        );

COMMENT ON COLUMN refresh_tokens.successor_id IS
    'UUID of the token issued to replace this one during rotation. '
    'NULL when the token has not been rotated (active or explicitly revoked). '
    'Non-NULL when the token was superseded by a rotation. '
    'Used exclusively as a boolean state marker — the value is never followed '
    'as a reference. Not a foreign key by design.';
