-- =============================================================================
-- Migration  : 000001_create_extensions_and_enums (DOWN)
-- Description: Drops all platform ENUM types and the pgcrypto extension.
--              This migration must only be run AFTER all subsequent migrations
--              have been rolled back (000016 → 000002 first), because any
--              table that references these types will block the DROP.
--              golang-migrate rolls back in reverse order automatically.
-- =============================================================================


-- Drop ENUM types in reverse creation order.
-- IF EXISTS guards against partial failures during development re-runs.
-- Do NOT use CASCADE — a type drop with CASCADE would silently drop every
-- column that uses the type, which is destructive and unrecoverable.

DROP TYPE IF EXISTS audit_action;

DROP TYPE IF EXISTS media_type;
DROP TYPE IF EXISTS media_entity_type;

DROP TYPE IF EXISTS match_event_type;
DROP TYPE IF EXISTS match_status;

DROP TYPE IF EXISTS registration_status;
DROP TYPE IF EXISTS participant_type;
DROP TYPE IF EXISTS tournament_format;
DROP TYPE IF EXISTS tournament_status;

DROP TYPE IF EXISTS membership_status;
DROP TYPE IF EXISTS membership_role;
DROP TYPE IF EXISTS team_status;

DROP TYPE IF EXISTS player_status;

DROP TYPE IF EXISTS role_scope;

DROP TYPE IF EXISTS org_type;
DROP TYPE IF EXISTS org_status;

DROP TYPE IF EXISTS gender;
DROP TYPE IF EXISTS user_status;

DROP EXTENSION IF EXISTS pgcrypto;
