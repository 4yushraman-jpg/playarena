-- =============================================================================
-- Migration  : 000012_create_media_attachments (UP)
-- Description: Creates the media_attachments table — a polymorphic store for
--              files attached to any domain entity (org, team, player, etc.).
--              entity_type + entity_id form a soft FK: PostgreSQL cannot enforce
--              a FK to multiple tables, so referential integrity is the
--              responsibility of the application service layer.
-- Depends on : 000002 (organizations), 000003 (users),
--              000001 (media_entity_type, media_type ENUMs)
-- =============================================================================

CREATE TABLE media_attachments (
    id              UUID              NOT NULL DEFAULT gen_random_uuid(),
    organization_id UUID              NOT NULL,
    entity_type     media_entity_type NOT NULL,
    entity_id       UUID              NOT NULL,
    media_type      media_type        NOT NULL,
    file_name       TEXT              NOT NULL,
    file_url        TEXT              NOT NULL,
    file_size       BIGINT,
    mime_type       TEXT,
    width           SMALLINT,
    height          SMALLINT,
    duration_secs   INTEGER,
    alt_text        TEXT,
    is_primary      BOOLEAN           NOT NULL DEFAULT FALSE,
    sort_order      SMALLINT          NOT NULL DEFAULT 0,
    uploaded_by     UUID,
    metadata        JSONB             NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ       NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ       NOT NULL DEFAULT NOW(),

    CONSTRAINT pk_media_attachments             PRIMARY KEY (id),
    CONSTRAINT fk_media_organization            FOREIGN KEY (organization_id)
                                                REFERENCES organizations (id) ON DELETE CASCADE,
    CONSTRAINT fk_media_uploaded_by             FOREIGN KEY (uploaded_by)
                                                REFERENCES users (id)         ON DELETE SET NULL,
    CONSTRAINT chk_media_file_name              CHECK (char_length(trim(file_name)) >= 1),
    CONSTRAINT chk_media_file_url               CHECK (char_length(trim(file_url)) >= 1),
    CONSTRAINT chk_media_file_size              CHECK (file_size IS NULL OR file_size > 0),
    -- Width and height must both be set or both be NULL (no half-dimensions)
    CONSTRAINT chk_media_dimensions             CHECK (
        (width IS NULL AND height IS NULL)
        OR (width IS NOT NULL AND height IS NOT NULL AND width > 0 AND height > 0)
    ),
    CONSTRAINT chk_media_duration_secs          CHECK (duration_secs IS NULL OR duration_secs > 0),
    CONSTRAINT chk_media_sort_order             CHECK (sort_order >= 0),
    -- Videos must declare a duration
    CONSTRAINT chk_media_video_has_duration     CHECK (
        media_type != 'video' OR duration_secs IS NOT NULL
    )
);

COMMENT ON TABLE media_attachments IS
    'Polymorphic media store. (entity_type, entity_id) is a soft FK to any domain entity. '
    'No database-level FK is possible across multiple tables: '
    'referential integrity is enforced by the application service layer. '
    'When a parent entity is deleted, the application must clean up its attachments. '
    'Physical file deletion from object storage is also the application''s responsibility.';

COMMENT ON COLUMN media_attachments.entity_type IS
    'Discriminator for the polymorphic reference. '
    'Paired with entity_id to identify the owning row in the correct domain table.';

COMMENT ON COLUMN media_attachments.entity_id IS
    'UUID of the entity this attachment belongs to. '
    'No DB-level FK — validated against the correct table by the service layer.';

COMMENT ON COLUMN media_attachments.is_primary IS
    'TRUE if this is the featured attachment for the entity '
    '(profile photo, team logo, tournament banner). '
    'At most one primary attachment per (entity_type, entity_id) should be active. '
    'Enforced by the application layer, not a DB constraint, to allow atomic swaps.';

COMMENT ON COLUMN media_attachments.sort_order IS
    'Display ordering within an entity''s attachment gallery. '
    'Lower values appear first. Default 0.';

COMMENT ON COLUMN media_attachments.metadata IS
    'CDN metadata, processing status, thumbnail variants, focal point hints, etc. '
    'Schema-free; validated at the application layer.';

COMMENT ON COLUMN media_attachments.alt_text IS
    'Accessibility text for images. Stored here rather than in a CMS.';

-- ---------------------------------------------------------------------------
-- Indexes
-- ---------------------------------------------------------------------------

-- Primary access pattern: "all media for entity X of type Y"
CREATE INDEX idx_media_entity          ON media_attachments (entity_type, entity_id);

-- Fast primary-only lookup (e.g. team logo, player photo)
CREATE INDEX idx_media_primary         ON media_attachments (entity_type, entity_id)
    WHERE is_primary = TRUE;

-- Org-wide media management panel
CREATE INDEX idx_media_organization_id ON media_attachments (organization_id);

-- Uploader history (e.g. "show media uploaded by user X")
CREATE INDEX idx_media_uploaded_by     ON media_attachments (uploaded_by)
    WHERE uploaded_by IS NOT NULL;
