-- =============================================================================
-- Migration  : 000020_media_hardening (UP)
-- Description: Hardens the media_attachments table established in 000012.
--
-- CHANGES
-- ───────────────────────────────────────────────────────────────────────────
-- Section 1 — Add storage_key and content_hash columns
--   storage_key: canonical S3 object path — source of truth, independent of
--                CDN domain. file_url is derived from storage_key + CDN base.
--   content_hash: SHA-256 hex digest of the raw uploaded bytes — enables
--                 duplicate detection and integrity verification.
--
-- Section 2 — Enforce one primary per entity at the DB level
--   UNIQUE partial index: at most one row per (entity_type, entity_id)
--   may have is_primary = TRUE. The application layer already enforces this
--   via a transactional swap, but the DB-level index provides a backstop.
--
-- Section 3 — Seed media.update and media.delete permissions
--   Grants both to the same roles that already hold media.upload.
--
-- PRODUCTION SAFETY NOTES
-- ───────────────────────────────────────────────────────────────────────────
-- The ADD COLUMN statements use NOT NULL DEFAULT '' / NOT NULL DEFAULT '0...'
-- to allow the migration to run safely on a table with existing rows.
-- The defaults are immediately dropped so future INSERTs must supply values.
-- Existing rows receive the placeholder defaults; if the table is non-empty
-- in production, a backfill script must be run BEFORE this migration.
--
-- Depends on : 000012 (media_attachments), 000017 (seed_rbac)
-- =============================================================================


-- =============================================================================
-- SECTION 1: ADD storage_key AND content_hash COLUMNS
-- =============================================================================

-- Step 1a: Add storage_key with a temporary default so existing rows are valid.
ALTER TABLE media_attachments
    ADD COLUMN IF NOT EXISTS storage_key TEXT NOT NULL DEFAULT '';

-- Step 1b: Add content_hash with a temporary all-zeros default.
-- 64 hex zeros is not a valid SHA-256 of any real file, making orphan rows
-- easy to identify in a post-migration audit.
ALTER TABLE media_attachments
    ADD COLUMN IF NOT EXISTS content_hash CHAR(64) NOT NULL
        DEFAULT '0000000000000000000000000000000000000000000000000000000000000000';

-- Step 1c: Drop the temporary defaults — new rows must always supply values.
ALTER TABLE media_attachments
    ALTER COLUMN storage_key DROP DEFAULT;

ALTER TABLE media_attachments
    ALTER COLUMN content_hash DROP DEFAULT;

-- Step 1d: Add CHECK constraints for both new columns.
ALTER TABLE media_attachments
    ADD CONSTRAINT chk_media_storage_key
        CHECK (char_length(trim(storage_key)) >= 1);

ALTER TABLE media_attachments
    ADD CONSTRAINT chk_media_content_hash
        CHECK (content_hash ~ '^[0-9a-f]{64}$');

COMMENT ON COLUMN media_attachments.storage_key IS
    'Canonical object path in the storage backend (e.g. S3 key). '
    'Source of truth — independent of CDN domain. file_url is derived from '
    'storage_key + the configured CDN base URL at insert time. '
    'When the CDN changes, update file_url from this column — not the reverse.';

COMMENT ON COLUMN media_attachments.content_hash IS
    'SHA-256 hex digest of the raw uploaded file bytes, computed before any '
    'processing. Used for duplicate detection: if (entity_type, entity_id, '
    'content_hash) already exists, the existing attachment is returned instead '
    'of creating a duplicate. Also used for integrity verification.';


-- =============================================================================
-- SECTION 2: DB-LEVEL PRIMARY UNIQUENESS GUARD
-- =============================================================================
-- The application already enforces is_primary uniqueness via a transactional
-- swap (FOR UPDATE lock → unset old → set new → COMMIT). This partial unique
-- index is a DB-level backstop that makes it structurally impossible to have
-- two primary attachments for the same entity, even if a future code path
-- bypasses the application-layer guard.

CREATE UNIQUE INDEX uq_media_primary_per_entity
    ON media_attachments (entity_type, entity_id)
    WHERE is_primary = TRUE;

COMMENT ON INDEX uq_media_primary_per_entity IS
    'Enforces that at most one media_attachments row per (entity_type, entity_id) '
    'may have is_primary = TRUE. The application layer uses a transactional swap '
    'to change the primary — this index is a DB-level backstop.';


-- =============================================================================
-- SECTION 3: SEED media.update AND media.delete PERMISSIONS
-- =============================================================================
-- 'update' and 'delete' are already in the chk_permissions_action vocabulary
-- (seeded in 000017). No constraint change is needed.

INSERT INTO permissions (name, slug, resource, action, description)
VALUES
    (
        'Update Media',
        'media.update',
        'media',
        'update',
        'Update media attachment metadata: alt text, sort order, and primary flag'
    ),
    (
        'Delete Media',
        'media.delete',
        'media',
        'delete',
        'Delete a media attachment and remove its file from object storage'
    )
ON CONFLICT (slug) DO NOTHING;

-- Grant both permissions to the same roles that already hold media.upload.
-- scorer and viewer are intentionally excluded — they have no write access.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM (VALUES
    ('platform_admin', 'media.update'),
    ('platform_admin', 'media.delete'),
    ('org_owner',      'media.update'),
    ('org_owner',      'media.delete'),
    ('org_admin',      'media.update'),
    ('org_admin',      'media.delete'),
    ('team_manager',   'media.update'),
    ('team_manager',   'media.delete'),
    ('coach',          'media.update'),
    ('coach',          'media.delete')
) AS mapping(role_slug, perm_slug)
JOIN roles       r ON r.slug = mapping.role_slug AND r.organization_id IS NULL
JOIN permissions p ON p.slug = mapping.perm_slug
ON CONFLICT DO NOTHING;
