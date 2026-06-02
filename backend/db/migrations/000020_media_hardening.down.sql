-- =============================================================================
-- Migration  : 000020_media_hardening (DOWN)
-- Description: Reverses 000020_media_hardening.up.sql
-- =============================================================================

-- Remove RBAC grants and permissions
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN ('media.update', 'media.delete')
);

DELETE FROM permissions WHERE slug IN ('media.update', 'media.delete');

-- Drop the primary uniqueness index
DROP INDEX IF EXISTS uq_media_primary_per_entity;

-- Drop constraints before dropping columns
ALTER TABLE media_attachments
    DROP CONSTRAINT IF EXISTS chk_media_content_hash;

ALTER TABLE media_attachments
    DROP CONSTRAINT IF EXISTS chk_media_storage_key;

-- Drop the new columns
ALTER TABLE media_attachments
    DROP COLUMN IF EXISTS content_hash;

ALTER TABLE media_attachments
    DROP COLUMN IF EXISTS storage_key;
