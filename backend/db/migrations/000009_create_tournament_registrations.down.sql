-- =============================================================================
-- Migration  : 000009_create_tournament_registrations (DOWN)
-- Description: Drops the tournament_registrations table and its indexes.
--              Partial unique indexes are dropped automatically with the table.
-- =============================================================================

DROP TABLE IF EXISTS tournament_registrations;
