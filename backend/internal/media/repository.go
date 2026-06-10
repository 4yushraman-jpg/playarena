package media

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
)

// Repository provides data-access for the media domain.
type Repository struct {
	queries *db.Queries
	pool    *pgxpool.Pool
}

// NewRepository constructs a Repository.
func NewRepository(queries *db.Queries, pool *pgxpool.Pool) *Repository {
	return &Repository{queries: queries, pool: pool}
}

// ── org resolution ─────────────────────────────────────────────────────────────

// GetOrgBySlug resolves an organization by its URL slug.
// Returns ErrOrganizationNotFound when the slug does not match any org.
func (r *Repository) GetOrgBySlug(ctx context.Context, slug string) (*db.Organization, error) {
	org, err := r.queries.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrOrganizationNotFound
		}
		return nil, err
	}
	return &org, nil
}

// ── reads ─────────────────────────────────────────────────────────────────────

func (r *Repository) GetByID(ctx context.Context, id, orgID pgtype.UUID) (*db.MediaAttachment, error) {
	a, err := r.queries.GetMediaAttachmentByID(ctx, db.GetMediaAttachmentByIDParams{
		ID:             id,
		OrganizationID: orgID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &a, nil
}

func (r *Repository) GetByContentHash(ctx context.Context, orgID pgtype.UUID, entityType db.MediaEntityType, entityID pgtype.UUID, hash string) (*db.MediaAttachment, error) {
	a, err := r.queries.GetMediaAttachmentByContentHash(ctx, db.GetMediaAttachmentByContentHashParams{
		OrganizationID: orgID,
		EntityType:     entityType,
		EntityID:       entityID,
		ContentHash:    hash,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &a, nil
}

func (r *Repository) List(ctx context.Context, orgID pgtype.UUID, params ListParams) ([]db.MediaAttachment, int64, error) {
	if params.Limit <= 0 || params.Limit > MaxListLimit {
		params.Limit = DefaultListLimit
	}
	if params.Offset < 0 {
		params.Offset = 0
	}

	// Entity-filtered listing.
	if params.EntityType != nil && params.EntityID != nil {
		entityType := db.MediaEntityType(*params.EntityType)
		entityID, err := pgutil.ParseUUID(*params.EntityID)
		if err != nil {
			return nil, 0, ErrInvalidEntityID
		}
		total, err := r.queries.CountMediaAttachmentsByEntity(ctx, db.CountMediaAttachmentsByEntityParams{
			OrganizationID: orgID,
			EntityType:     entityType,
			EntityID:       entityID,
		})
		if err != nil {
			return nil, 0, err
		}
		rows, err := r.queries.ListMediaAttachmentsByEntity(ctx, db.ListMediaAttachmentsByEntityParams{
			OrganizationID: orgID,
			EntityType:     entityType,
			EntityID:       entityID,
			Limit:          params.Limit,
			Offset:         params.Offset,
		})
		return rows, total, err
	}

	// Org-wide listing.
	total, err := r.queries.CountAllMediaByOrg(ctx, orgID)
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.queries.ListAllMediaByOrg(ctx, db.ListAllMediaByOrgParams{
		OrganizationID: orgID,
		Limit:          params.Limit,
		Offset:         params.Offset,
	})
	return rows, total, err
}

// ── entity existence checks ───────────────────────────────────────────────────

func (r *Repository) PlayerExists(ctx context.Context, playerID, orgID pgtype.UUID) (bool, error) {
	return r.queries.MediaCheckPlayerExists(ctx, db.MediaCheckPlayerExistsParams{
		ID:             playerID,
		OrganizationID: orgID,
	})
}

func (r *Repository) TeamExists(ctx context.Context, teamID, orgID pgtype.UUID) (bool, error) {
	return r.queries.MediaCheckTeamExists(ctx, db.MediaCheckTeamExistsParams{
		ID:             teamID,
		OrganizationID: orgID,
	})
}

func (r *Repository) TournamentExists(ctx context.Context, tournamentID, orgID pgtype.UUID) (bool, error) {
	return r.queries.MediaCheckTournamentExists(ctx, db.MediaCheckTournamentExistsParams{
		ID:             tournamentID,
		OrganizationID: orgID,
	})
}

func (r *Repository) MatchExists(ctx context.Context, matchID, orgID pgtype.UUID) (bool, error) {
	_, err := r.queries.GetMatchByID(ctx, db.GetMatchByIDParams{
		ID:             matchID,
		OrganizationID: orgID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *Repository) UserExists(ctx context.Context, userID pgtype.UUID) (bool, error) {
	_, err := r.queries.GetUserByID(ctx, userID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ── writes (transactional) ────────────────────────────────────────────────────

// createMediaTxParams bundles inputs for CreateWithAudit.
type createMediaTxParams struct {
	attachment db.CreateMediaAttachmentParams
	actorID    pgtype.UUID
	orgID      pgtype.UUID
}

// CreateWithAudit atomically inserts a new media_attachments row and an audit
// log record. The is_primary swap (if needed) is handled separately via
// SwapPrimaryWithAudit after this transaction commits.
//
// Defect 2 fix: if the INSERT fails with a unique violation on
// uq_media_content_per_entity (concurrent duplicate upload race), the
// transaction is rolled back and the existing attachment is returned instead.
func (r *Repository) CreateWithAudit(ctx context.Context, p createMediaTxParams) (*db.MediaAttachment, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	row, err := qtx.CreateMediaAttachment(ctx, p.attachment)
	if err != nil {
		// Defect 2: concurrent duplicate upload — second INSERT hits the unique
		// index on (organization_id, entity_type, entity_id, content_hash).
		// The transaction is rolled back (defer handles it); re-query to find
		// and return the existing row committed by the concurrent request.
		if pgutil.IsUniqueViolation(err, "uq_media_content_per_entity") {
			existing, reErr := r.queries.GetMediaAttachmentByContentHash(ctx,
				db.GetMediaAttachmentByContentHashParams{
					OrganizationID: p.attachment.OrganizationID,
					EntityType:     p.attachment.EntityType,
					EntityID:       p.attachment.EntityID,
					ContentHash:    p.attachment.ContentHash,
				})
			if reErr != nil {
				// Re-query failed — return the original unique violation error.
				return nil, err
			}
			return &existing, nil
		}
		return nil, err
	}

	newData, err := attachmentToAuditJSON(&row)
	if err != nil {
		return nil, err
	}

	if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
		OrganizationID: p.orgID,
		UserID:         p.actorID,
		Action:         db.AuditActionCreate,
		EntityType:     "media_attachments",
		EntityID:       row.ID,
		NewData:        newData,
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &row, nil
}

// updateMediaTxParams bundles inputs for UpdateWithAudit.
type updateMediaTxParams struct {
	id        pgtype.UUID
	orgID     pgtype.UUID
	actorID   pgtype.UUID
	altText   *string
	sortOrder int16
	oldData   []byte
}

// UpdateWithAudit atomically updates mutable metadata and appends an audit
// record with old_data / new_data snapshots.
func (r *Repository) UpdateWithAudit(ctx context.Context, p updateMediaTxParams) (*db.MediaAttachment, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	row, err := qtx.UpdateMediaAttachmentMeta(ctx, db.UpdateMediaAttachmentMetaParams{
		ID:             p.id,
		OrganizationID: p.orgID,
		AltText:        p.altText,
		SortOrder:      p.sortOrder,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	newData, err := attachmentToAuditJSON(&row)
	if err != nil {
		return nil, err
	}

	if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
		OrganizationID: p.orgID,
		UserID:         p.actorID,
		Action:         db.AuditActionUpdate,
		EntityType:     "media_attachments",
		EntityID:       row.ID,
		OldData:        p.oldData,
		NewData:        newData,
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &row, nil
}

// swapPrimaryTxParams bundles inputs for SwapPrimaryWithAudit.
type swapPrimaryTxParams struct {
	newPrimaryID pgtype.UUID
	orgID        pgtype.UUID
	entityType   db.MediaEntityType
	entityID     pgtype.UUID
	actorID      pgtype.UUID
}

// SwapPrimaryWithAudit atomically:
//  1. Acquires FOR UPDATE lock on the current primary (if any) — serialises
//     concurrent swap requests for the same entity.
//  2. Unsets is_primary on the old primary, auditing the change with both
//     old_data and new_data.
//  3. Fetches the pre-update snapshot of the new attachment.
//  4. Sets is_primary on the new attachment.
//  5. Audits the new primary change with old_data (pre-swap state) and
//     new_data (post-swap state), satisfying chk_audit_update_has_both_snapshots.
//
// Defect 1 fix: step 3 captures the attachment's state before SetAttachmentAsPrimary
// so the second audit record's OldData is non-nil. Previously it was nil, which
// always caused the transaction to roll back on the audit constraint check.
func (r *Repository) SwapPrimaryWithAudit(ctx context.Context, p swapPrimaryTxParams) (*db.MediaAttachment, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	// Step 1: lock the current primary row (FOR UPDATE serialises concurrent swaps).
	oldPrimary, lockErr := qtx.LockPrimaryMediaAttachment(ctx, db.LockPrimaryMediaAttachmentParams{
		OrganizationID: p.orgID,
		EntityType:     p.entityType,
		EntityID:       p.entityID,
	})

	// Step 2: if there was an old primary, unset it and audit the change.
	if lockErr == nil {
		oldSnap, snapErr := attachmentToAuditJSON(&oldPrimary)
		if snapErr != nil {
			return nil, snapErr
		}

		if err := qtx.UnsetPrimaryForEntity(ctx, db.UnsetPrimaryForEntityParams{
			OrganizationID: p.orgID,
			EntityType:     p.entityType,
			EntityID:       p.entityID,
		}); err != nil {
			return nil, err
		}

		unsetNew, _ := json.Marshal(map[string]any{
			"id":         pgutil.UUIDToString(oldPrimary.ID),
			"is_primary": false,
		})
		if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
			OrganizationID: p.orgID,
			UserID:         p.actorID,
			Action:         db.AuditActionUpdate,
			EntityType:     "media_attachments",
			EntityID:       oldPrimary.ID,
			OldData:        oldSnap,
			NewData:        unsetNew,
		}); err != nil {
			return nil, err
		}
	} else if lockErr != pgx.ErrNoRows {
		return nil, lockErr
	}

	// Step 3: capture the pre-update state of the new attachment.
	// This is the OldData for the audit record that follows SetAttachmentAsPrimary.
	// Defect 1 fix: without this snapshot, old_data was nil and the audit INSERT
	// violated chk_audit_update_has_both_snapshots, rolling back every swap.
	preSwap, err := qtx.GetMediaAttachmentByID(ctx, db.GetMediaAttachmentByIDParams{
		ID:             p.newPrimaryID,
		OrganizationID: p.orgID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	preSwapSnap, err := attachmentToAuditJSON(&preSwap)
	if err != nil {
		return nil, err
	}

	// Step 4: set the new primary.
	newPrimary, err := qtx.SetAttachmentAsPrimary(ctx, db.SetAttachmentAsPrimaryParams{
		ID:             p.newPrimaryID,
		OrganizationID: p.orgID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	// Step 5: audit the new primary being set, with both old and new snapshots.
	newSnap, err := attachmentToAuditJSON(&newPrimary)
	if err != nil {
		return nil, err
	}
	if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
		OrganizationID: p.orgID,
		UserID:         p.actorID,
		Action:         db.AuditActionUpdate,
		EntityType:     "media_attachments",
		EntityID:       newPrimary.ID,
		OldData:        preSwapSnap, // pre-swap state (is_primary=false)
		NewData:        newSnap,     // post-swap state (is_primary=true)
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &newPrimary, nil
}

// deleteMediaTxParams bundles inputs for DeleteWithAudit.
type deleteMediaTxParams struct {
	id      pgtype.UUID
	orgID   pgtype.UUID
	actorID pgtype.UUID
	oldData []byte
}

// DeleteWithAudit atomically deletes a media_attachments row and appends an
// audit record. Storage object deletion happens AFTER this commit — it is
// outside the transaction boundary by design.
//
// Defect 3 fix: DeleteMediaAttachment now returns rows affected (via :execrows).
// If zero rows are deleted the attachment was already gone (concurrent delete);
// ErrNotFound is returned and no audit record is written.
func (r *Repository) DeleteWithAudit(ctx context.Context, p deleteMediaTxParams) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	rowsAffected, err := qtx.DeleteMediaAttachment(ctx, db.DeleteMediaAttachmentParams{
		ID:             p.id,
		OrganizationID: p.orgID,
	})
	if err != nil {
		return err
	}
	// Defect 3 fix: zero rows affected means a concurrent request already deleted
	// this attachment. Do not write a spurious audit record.
	if rowsAffected == 0 {
		return ErrNotFound
	}

	if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
		OrganizationID: p.orgID,
		UserID:         p.actorID,
		Action:         db.AuditActionDelete,
		EntityType:     "media_attachments",
		EntityID:       p.id,
		OldData:        p.oldData,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func attachmentToAuditJSON(a *db.MediaAttachment) ([]byte, error) {
	return json.Marshal(map[string]any{
		"id":           pgutil.UUIDToString(a.ID),
		"entity_type":  string(a.EntityType),
		"entity_id":    pgutil.UUIDToString(a.EntityID),
		"media_type":   string(a.MediaType),
		"file_name":    a.FileName,
		"file_url":     a.FileUrl,
		"storage_key":  a.StorageKey,
		"content_hash": a.ContentHash,
		"file_size":    a.FileSize,
		"mime_type":    a.MimeType,
		"width":        a.Width,
		"height":       a.Height,
		"is_primary":   a.IsPrimary,
		"sort_order":   a.SortOrder,
		"created_at":   a.CreatedAt.Time.UTC().Format(time.RFC3339),
		"updated_at":   a.UpdatedAt.Time.UTC().Format(time.RFC3339),
	})
}
