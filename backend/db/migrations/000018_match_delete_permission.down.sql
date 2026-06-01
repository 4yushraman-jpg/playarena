-- =============================================================================
-- Migration  : 000018_match_delete_permission (DOWN)
-- Description: Removes the match.delete permission and its role grants.
--              Reverses 000018_match_delete_permission.up.sql exactly.
-- =============================================================================

-- Remove the three role_permissions rows for match.delete
DELETE FROM role_permissions
WHERE permission_id = (
    SELECT id FROM permissions WHERE slug = 'match.delete'
);

-- Remove the permission row itself
DELETE FROM permissions
WHERE slug = 'match.delete';
