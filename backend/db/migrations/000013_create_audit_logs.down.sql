-- =============================================================================
-- Migration  : 000013_create_audit_logs (DOWN)
-- Description: Drops the audit_logs table and all its indexes.
--              This is an independent table with no downstream FK dependents,
--              so it can be dropped at any point in the rollback sequence.
-- =============================================================================

DROP TABLE IF EXISTS audit_logs;
