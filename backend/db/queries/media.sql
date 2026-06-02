-- =============================================================================
-- Queries : media_attachments
-- Module  : internal/media
-- =============================================================================

-- ─────────────────────────────────────────────────────────────────────────────
-- WRITES
-- ─────────────────────────────────────────────────────────────────────────────

-- name: CreateMediaAttachment :one
INSERT INTO media_attachments (
    organization_id,
    entity_type,
    entity_id,
    media_type,
    file_name,
    file_url,
    storage_key,
    content_hash,
    file_size,
    mime_type,
    width,
    height,
    alt_text,
    is_primary,
    sort_order,
    uploaded_by,
    metadata
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17
)
RETURNING *;

-- name: UpdateMediaAttachmentMeta :one
-- Updates mutable metadata fields only (not storage fields or entity references).
UPDATE media_attachments
SET
    alt_text   = $3,
    sort_order = $4,
    updated_at = NOW()
WHERE id              = $1
  AND organization_id = $2
RETURNING *;

-- name: SetAttachmentAsPrimary :one
-- Called inside the primary-swap transaction AFTER unsetting the old primary.
UPDATE media_attachments
SET
    is_primary = TRUE,
    updated_at = NOW()
WHERE id              = $1
  AND organization_id = $2
RETURNING *;

-- name: UnsetPrimaryForEntity :exec
-- Clears the is_primary flag for all attachments belonging to an entity.
-- Called inside the primary-swap transaction before setting the new primary.
UPDATE media_attachments
SET
    is_primary = FALSE,
    updated_at = NOW()
WHERE organization_id = $1
  AND entity_type     = $2
  AND entity_id       = $3
  AND is_primary      = TRUE;

-- name: DeleteMediaAttachment :execrows
DELETE FROM media_attachments
WHERE id              = $1
  AND organization_id = $2;

-- ─────────────────────────────────────────────────────────────────────────────
-- READS
-- ─────────────────────────────────────────────────────────────────────────────

-- name: GetMediaAttachmentByID :one
-- org-scoped lookup: prevents cross-tenant reads (BOLA protection).
SELECT * FROM media_attachments
WHERE id              = $1
  AND organization_id = $2
LIMIT 1;

-- name: GetMediaAttachmentByContentHash :one
-- Duplicate detection: same entity + same content hash = existing record.
SELECT * FROM media_attachments
WHERE organization_id = $1
  AND entity_type     = $2
  AND entity_id       = $3
  AND content_hash    = $4
LIMIT 1;

-- name: LockPrimaryMediaAttachment :one
-- Acquires FOR UPDATE row lock on the current primary attachment for an entity.
-- Used in the primary-swap transaction to serialise concurrent swap requests.
-- Returns NULL (pgx.ErrNoRows) when no primary exists for the entity.
SELECT * FROM media_attachments
WHERE organization_id = $1
  AND entity_type     = $2
  AND entity_id       = $3
  AND is_primary      = TRUE
LIMIT 1
FOR UPDATE;

-- name: ListMediaAttachmentsByEntity :many
-- Returns all attachments for a specific entity, primary first, then by sort
-- order ascending, then newest first within the same sort_order.
SELECT * FROM media_attachments
WHERE organization_id = $1
  AND entity_type     = $2
  AND entity_id       = $3
ORDER BY is_primary DESC, sort_order ASC, created_at DESC
LIMIT  $4
OFFSET $5;

-- name: CountMediaAttachmentsByEntity :one
SELECT COUNT(*) FROM media_attachments
WHERE organization_id = $1
  AND entity_type     = $2
  AND entity_id       = $3;

-- name: ListAllMediaByOrg :many
-- Org-wide listing (no entity filter). Used for the media management panel.
SELECT * FROM media_attachments
WHERE organization_id = $1
ORDER BY created_at DESC
LIMIT  $2
OFFSET $3;

-- name: CountAllMediaByOrg :one
SELECT COUNT(*) FROM media_attachments
WHERE organization_id = $1;

-- ─────────────────────────────────────────────────────────────────────────────
-- ENTITY EXISTENCE VALIDATION
-- Validates that the target entity exists in the actor's org before upload.
-- These are direct EXISTS queries to avoid importing other module packages.
-- ─────────────────────────────────────────────────────────────────────────────

-- name: MediaCheckPlayerExists :one
SELECT EXISTS(
    SELECT 1 FROM players
    WHERE id              = $1
      AND organization_id = $2
)::boolean;

-- name: MediaCheckTeamExists :one
SELECT EXISTS(
    SELECT 1 FROM teams
    WHERE id              = $1
      AND organization_id = $2
)::boolean;

-- name: MediaCheckTournamentExists :one
SELECT EXISTS(
    SELECT 1 FROM tournaments
    WHERE id              = $1
      AND organization_id = $2
)::boolean;
