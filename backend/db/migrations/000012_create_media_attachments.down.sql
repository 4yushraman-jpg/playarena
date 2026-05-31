-- =============================================================================
-- Migration  : 000012_create_media_attachments (DOWN)
-- Description: Drops the media_attachments table and all its indexes.
-- =============================================================================

DROP TABLE IF EXISTS media_attachments;
