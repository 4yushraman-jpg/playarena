-- =============================================================================
-- Migration  : 000021_media_content_uniqueness (UP)
-- Description: Adds a unique index on (organization_id, entity_type,
--              entity_id, content_hash) to prevent concurrent duplicate uploads
--              from creating multiple rows for the same content.
--
-- The application layer performs a GetByContentHash check before inserting,
-- but that check is non-atomic. Two concurrent requests can both observe
-- ErrNotFound and both proceed to insert. This index provides the DB-level
-- backstop: the second INSERT fails with SQLSTATE 23505, which the repository
-- detects and handles by re-querying for the existing row.
--
-- Depends on : 000020 (media_attachments, storage_key, content_hash columns)
-- =============================================================================

CREATE UNIQUE INDEX uq_media_content_per_entity
    ON media_attachments (organization_id, entity_type, entity_id, content_hash);

COMMENT ON INDEX uq_media_content_per_entity IS
    'Prevents duplicate content within the same entity. '
    'If two concurrent uploads of the same file (same SHA-256 hash) race '
    'past the application-layer GetByContentHash check, the second INSERT '
    'fails with a unique violation (SQLSTATE 23505). The repository catches '
    'this and returns the existing attachment instead of surfacing an error.';
