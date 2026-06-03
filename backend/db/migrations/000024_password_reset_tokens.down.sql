-- =============================================================================
-- Migration  : 000024_password_reset_tokens (DOWN)
-- Description: Removes the password_reset_tokens table introduced in the UP
--              migration. CASCADE drops the two indexes automatically.
-- =============================================================================

DROP TABLE IF EXISTS password_reset_tokens CASCADE;
