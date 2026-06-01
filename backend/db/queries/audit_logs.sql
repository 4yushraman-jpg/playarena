-- Audit log queries
-- The audit_logs table is append-only: no UPDATE or DELETE is ever performed.

-- name: CreateAuditLog :exec
-- Inserts one immutable audit record. Caller is responsible for satisfying
-- the table CHECK constraints:
--   • create/update/delete/permission_change: entity_id must be non-NULL
--   • update: both old_data and new_data must be non-NULL
--   • login/logout: entity_id must be NULL
INSERT INTO audit_logs (
    organization_id,
    user_id,
    action,
    entity_type,
    entity_id,
    old_data,
    new_data,
    ip_address,
    user_agent
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);
