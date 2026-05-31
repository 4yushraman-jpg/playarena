-- =============================================================================
-- Migration  : 000002_create_organizations (DOWN)
-- Description: Drops the organizations table and all its indexes.
--              Run AFTER rolling back all migrations that depend on this table
--              (000003 through 000013).
-- =============================================================================

DROP TABLE IF EXISTS organizations;
