package media

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/media/processor"
	"github.com/4yushraman-jpg/playarena/internal/media/storage"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
)

// supportedEntityTypes lists entity types supported in Phase 11.
// "match" and "user" are deferred.
var supportedEntityTypes = map[string]db.MediaEntityType{
	"organization": db.MediaEntityTypeOrganization,
	"team":         db.MediaEntityTypeTeam,
	"player":       db.MediaEntityTypePlayer,
	"tournament":   db.MediaEntityTypeTournament,
}

// Service implements media use-cases.
type Service struct {
	repo    *Repository
	backend storage.Backend
	log     *slog.Logger
}

// NewService constructs a Service.
func NewService(repo *Repository, backend storage.Backend, log *slog.Logger) *Service {
	return &Service{repo: repo, backend: backend, log: log}
}

// ── Upload ─────────────────────────────────────────────────────────────────────

// Upload validates, processes, stores, and records a new media attachment.
//
// Pipeline:
//  1. Validate entity type (Phase 11 allow-list)
//  2. Resolve entity UUID
//  3. Resolve org by slug (BOLA check)
//  4. Verify entity belongs to actor's org
//  5. Process image (MIME validate → decode → WebP → thumbnails → hash)
//  6. Duplicate detection via content hash
//  7. Upload all variants to storage backend
//  8. Insert DB row + audit log (same transaction)
//  9. Swap is_primary (separate transaction if requested)
func (s *Service) Upload(
	ctx context.Context,
	orgSlug string,
	entityTypeStr string,
	entityIDStr string,
	altText string,
	isPrimary bool,
	originalFileName string,
	fileData []byte,
	actorUserID string,
	actorOrgID string,
) (*Response, error) {
	// ── 1. Validate entity type ──────────────────────────────────────────────
	entityTypeKey := strings.ToLower(strings.TrimSpace(entityTypeStr))
	entityType, ok := supportedEntityTypes[entityTypeKey]
	if !ok {
		return nil, ErrUnsupportedEntityType
	}

	// ── 2. Resolve entity UUID ───────────────────────────────────────────────
	entityID, err := pgutil.ParseUUID(entityIDStr)
	if err != nil {
		return nil, ErrInvalidEntityID
	}

	// ── 3. Resolve org + BOLA ────────────────────────────────────────────────
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}
	if err := assertOrgOwnership(actorOrgID, pgutil.UUIDToString(org.ID)); err != nil {
		return nil, err
	}
	orgID := org.ID

	actorUID, err := pgutil.ParseUUID(actorUserID)
	if err != nil {
		return nil, errors.New("media: invalid actor user id")
	}

	// ── 4. Verify entity belongs to this org ─────────────────────────────────
	if err := s.assertEntityExists(ctx, entityType, entityID, orgID); err != nil {
		return nil, err
	}

	// ── 5. Process image ──────────────────────────────────────────────────────
	result, err := processor.Process(bytes.NewReader(fileData), originalFileName)
	if err != nil {
		if errors.Is(err, processor.ErrUnsupportedMIME) {
			return nil, ErrUnsupportedMIME
		}
		return nil, fmt.Errorf("media: image processing failed: %w", err)
	}

	// ── 6. Duplicate detection ────────────────────────────────────────────────
	existing, err := s.repo.GetByContentHash(ctx, orgID, entityType, entityID, result.ContentHash)
	if err == nil {
		// Identical content already exists for this entity — return it idempotently.
		return s.attachmentToResponse(existing), nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	// ── 7. Upload variants to storage ─────────────────────────────────────────
	fileUUID := generateFileUUID()
	orgIDStr := pgutil.UUIDToString(orgID)
	entityIDStr2 := pgutil.UUIDToString(entityID)

	fullKey := storage.GenerateKey(orgIDStr, entityTypeKey, entityIDStr2, fileUUID, processor.OutputSuffix)
	smKey := storage.GenerateKey(orgIDStr, entityTypeKey, entityIDStr2, fileUUID, processor.ThumbSMSuffix)
	mdKey := storage.GenerateKey(orgIDStr, entityTypeKey, entityIDStr2, fileUUID, processor.ThumbMDSuffix)

	if err := s.backend.Upload(ctx, fullKey, bytes.NewReader(result.Full.Data), int64(len(result.Full.Data)), processor.OutputMIME); err != nil {
		return nil, fmt.Errorf("media: upload full-size: %w", err)
	}
	if err := s.backend.Upload(ctx, smKey, bytes.NewReader(result.ThumbSM.Data), int64(len(result.ThumbSM.Data)), processor.OutputMIME); err != nil {
		_ = s.backend.Delete(ctx, fullKey)
		return nil, fmt.Errorf("media: upload sm thumbnail: %w", err)
	}
	if err := s.backend.Upload(ctx, mdKey, bytes.NewReader(result.ThumbMD.Data), int64(len(result.ThumbMD.Data)), processor.OutputMIME); err != nil {
		_ = s.backend.Delete(ctx, fullKey)
		_ = s.backend.Delete(ctx, smKey)
		return nil, fmt.Errorf("media: upload md thumbnail: %w", err)
	}

	// ── 8. Build metadata JSONB and DB row ────────────────────────────────────
	metadataJSON, _ := json.Marshal(map[string]any{
		"processing_status": "done",
		"variants":          VariantsMeta{FullKey: fullKey, SMKey: smKey, MDKey: mdKey},
		"original_filename": result.OriginalFileName,
		"original_mime":     result.DetectedMIME,
	})

	fileURL := s.backend.GetPublicURL(fullKey)
	fileSize := int64(len(result.Full.Data))
	mimeType := processor.OutputMIME
	w := int16(result.Width)
	h := int16(result.Height)

	attachment, err := s.repo.CreateWithAudit(ctx, createMediaTxParams{
		orgID:   orgID,
		actorID: actorUID,
		attachment: db.CreateMediaAttachmentParams{
			OrganizationID: orgID,
			EntityType:     entityType,
			EntityID:       entityID,
			MediaType:      db.MediaTypeImage,
			FileName:       result.OriginalFileName,
			FileUrl:        fileURL,
			StorageKey:     fullKey,
			ContentHash:    result.ContentHash,
			FileSize:       &fileSize,
			MimeType:       &mimeType,
			Width:          &w,
			Height:         &h,
			AltText:        nullableString(altText),
			IsPrimary:      false, // set via swap after insert
			SortOrder:      0,
			UploadedBy:     actorUID,
			Metadata:       metadataJSON,
		},
	})
	if err != nil {
		_ = s.backend.Delete(ctx, fullKey)
		_ = s.backend.Delete(ctx, smKey)
		_ = s.backend.Delete(ctx, mdKey)
		return nil, err
	}

	// ── 9. Swap is_primary (separate transaction) ─────────────────────────────
	// Defect 1 fix: propagate swap errors instead of silently discarding them.
	if isPrimary {
		swapped, swapErr := s.repo.SwapPrimaryWithAudit(ctx, swapPrimaryTxParams{
			newPrimaryID: attachment.ID,
			orgID:        orgID,
			entityType:   entityType,
			entityID:     entityID,
			actorID:      actorUID,
		})
		if swapErr != nil {
			return nil, swapErr
		}
		attachment = swapped
	}

	return s.attachmentToResponse(attachment), nil
}

// ── List ───────────────────────────────────────────────────────────────────────

func (s *Service) List(ctx context.Context, orgSlug string, params ListParams, actorOrgID string) (*ListResponse, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}
	if err := assertOrgOwnership(actorOrgID, pgutil.UUIDToString(org.ID)); err != nil {
		return nil, err
	}

	rows, total, err := s.repo.List(ctx, org.ID, params)
	if err != nil {
		return nil, err
	}
	resp := make([]Response, len(rows))
	for i := range rows {
		resp[i] = *s.attachmentToResponse(&rows[i])
	}
	return &ListResponse{
		Attachments: resp,
		Total:       total,
		Limit:       params.Limit,
		Offset:      params.Offset,
	}, nil
}

// ── GetByID ────────────────────────────────────────────────────────────────────

func (s *Service) GetByID(ctx context.Context, orgSlug, idStr string, actorOrgID string) (*Response, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}
	if err := assertOrgOwnership(actorOrgID, pgutil.UUIDToString(org.ID)); err != nil {
		return nil, err
	}

	id, err := pgutil.ParseUUID(idStr)
	if err != nil {
		return nil, ErrNotFound
	}

	a, err := s.repo.GetByID(ctx, id, org.ID)
	if err != nil {
		return nil, err
	}
	return s.attachmentToResponse(a), nil
}

// ── Update ─────────────────────────────────────────────────────────────────────

func (s *Service) Update(ctx context.Context, orgSlug, idStr string, req UpdateRequest, actorUserID, actorOrgID string) (*Response, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}
	if err := assertOrgOwnership(actorOrgID, pgutil.UUIDToString(org.ID)); err != nil {
		return nil, err
	}

	id, err := pgutil.ParseUUID(idStr)
	if err != nil {
		return nil, ErrNotFound
	}

	actorUID, err := pgutil.ParseUUID(actorUserID)
	if err != nil {
		return nil, errors.New("media: invalid actor user id")
	}

	current, err := s.repo.GetByID(ctx, id, org.ID)
	if err != nil {
		return nil, err
	}

	oldData, err := attachmentToAuditJSON(current)
	if err != nil {
		return nil, err
	}

	// Merge: apply only non-nil fields.
	altText := current.AltText
	if req.AltText != nil {
		altText = req.AltText
	}
	sortOrder := current.SortOrder
	if req.SortOrder != nil {
		sortOrder = *req.SortOrder
	}

	updated, err := s.repo.UpdateWithAudit(ctx, updateMediaTxParams{
		id:        id,
		orgID:     org.ID,
		actorID:   actorUID,
		altText:   altText,
		sortOrder: sortOrder,
		oldData:   oldData,
	})
	if err != nil {
		return nil, err
	}

	// Handle is_primary swap separately.
	// Defect 1 fix: propagate swap errors instead of silently discarding them.
	if req.IsPrimary != nil && *req.IsPrimary && !updated.IsPrimary {
		swapped, swapErr := s.repo.SwapPrimaryWithAudit(ctx, swapPrimaryTxParams{
			newPrimaryID: updated.ID,
			orgID:        org.ID,
			entityType:   updated.EntityType,
			entityID:     updated.EntityID,
			actorID:      actorUID,
		})
		if swapErr != nil {
			return nil, swapErr
		}
		updated = swapped
	}

	return s.attachmentToResponse(updated), nil
}

// ── Delete ─────────────────────────────────────────────────────────────────────

func (s *Service) Delete(ctx context.Context, orgSlug, idStr string, actorUserID, actorOrgID string) error {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return err
	}
	if err := assertOrgOwnership(actorOrgID, pgutil.UUIDToString(org.ID)); err != nil {
		return err
	}

	id, err := pgutil.ParseUUID(idStr)
	if err != nil {
		return ErrNotFound
	}

	actorUID, err := pgutil.ParseUUID(actorUserID)
	if err != nil {
		return errors.New("media: invalid actor user id")
	}

	current, err := s.repo.GetByID(ctx, id, org.ID)
	if err != nil {
		return err
	}

	oldData, err := attachmentToAuditJSON(current)
	if err != nil {
		return err
	}

	// Delete DB row + audit record (transactional).
	if err := s.repo.DeleteWithAudit(ctx, deleteMediaTxParams{
		id:      id,
		orgID:   org.ID,
		actorID: actorUID,
		oldData: oldData,
	}); err != nil {
		return err
	}

	// Delete storage objects AFTER the transaction commits.
	// Failures are logged; orphaned files are handled by reconciliation.
	if current.StorageKey != "" {
		if delErr := s.backend.Delete(ctx, current.StorageKey); delErr != nil {
			s.log.ErrorContext(ctx, "media.delete.storage_failed",
				slog.String("key", current.StorageKey),
				slog.Any("error", delErr),
			)
		}
		var meta map[string]any
		if json.Unmarshal(current.Metadata, &meta) == nil {
			if variantsRaw, ok := meta["variants"].(map[string]any); ok {
				for _, v := range variantsRaw {
					if key, ok := v.(string); ok && key != "" && key != current.StorageKey {
						_ = s.backend.Delete(ctx, key)
					}
				}
			}
		}
	}

	return nil
}

// ── helpers ────────────────────────────────────────────────────────────────────

func (s *Service) attachmentToResponse(a *db.MediaAttachment) *Response {
	r := &Response{
		ID:         pgutil.UUIDToString(a.ID),
		EntityType: string(a.EntityType),
		EntityID:   pgutil.UUIDToString(a.EntityID),
		MediaType:  string(a.MediaType),
		FileName:   a.FileName,
		FileSize:   a.FileSize,
		MimeType:   a.MimeType,
		Width:      a.Width,
		Height:     a.Height,
		AltText:    a.AltText,
		IsPrimary:  a.IsPrimary,
		SortOrder:  a.SortOrder,
		FileURL:    a.FileUrl,
		CreatedAt:  a.CreatedAt.Time.UTC().Format(time.RFC3339),
		UpdatedAt:  a.UpdatedAt.Time.UTC().Format(time.RFC3339),
	}
	if a.UploadedBy.Valid {
		uid := pgutil.UUIDToString(a.UploadedBy)
		r.UploadedBy = &uid
	}
	return r
}

// assertEntityExists verifies the target entity exists in the given org.
// For "organization" type, entity_id must equal orgID.
func (s *Service) assertEntityExists(ctx context.Context, entityType db.MediaEntityType, entityID, orgID pgtype.UUID) error {
	switch entityType {
	case db.MediaEntityTypeOrganization:
		// Media for an org entity can only be attached to the actor's own org.
		if pgutil.UUIDToString(entityID) != pgutil.UUIDToString(orgID) {
			return ErrEntityNotFound
		}
		return nil
	case db.MediaEntityTypePlayer:
		ok, err := s.repo.PlayerExists(ctx, entityID, orgID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrEntityNotFound
		}
	case db.MediaEntityTypeTeam:
		ok, err := s.repo.TeamExists(ctx, entityID, orgID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrEntityNotFound
		}
	case db.MediaEntityTypeTournament:
		ok, err := s.repo.TournamentExists(ctx, entityID, orgID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrEntityNotFound
		}
	default:
		return ErrUnsupportedEntityType
	}
	return nil
}

// assertOrgOwnership is the standard BOLA guard. Platform admins (empty
// actorOrgID) are unconditionally allowed.
func assertOrgOwnership(actorOrgID, targetOrgID string) error {
	if actorOrgID == "" {
		return nil
	}
	if actorOrgID != targetOrgID {
		return ErrForbidden
	}
	return nil
}

func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// generateFileUUID produces a cryptographically random UUID v4 string.
// Used as the unique filename component of storage keys.
func generateFileUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant RFC 4122
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
