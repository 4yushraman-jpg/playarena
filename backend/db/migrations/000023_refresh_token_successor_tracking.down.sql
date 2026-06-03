-- =============================================================================
-- Migration  : 000023_refresh_token_successor_tracking (DOWN)
-- Description: Reverses the successor_id column and its CHECK constraint.
-- =============================================================================

ALTER TABLE refresh_tokens
    DROP CONSTRAINT chk_refresh_tokens_successor;

ALTER TABLE refresh_tokens
    DROP COLUMN successor_id;
