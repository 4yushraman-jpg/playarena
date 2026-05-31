-- =============================================================================
-- Migration  : 000003_create_users (DOWN)
-- Description: Drops refresh_tokens then users.
--              Order matters: refresh_tokens FKs into users.
-- =============================================================================

DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS users;
