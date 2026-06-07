package notifications

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

// Repository provides data access for the notifications domain.
type Repository struct {
	queries *db.Queries
	pool    *pgxpool.Pool
}

// NewRepository constructs a Repository.
func NewRepository(queries *db.Queries, pool *pgxpool.Pool) *Repository {
	return &Repository{queries: queries, pool: pool}
}

// ── reads ─────────────────────────────────────────────────────────────────────

// GetOrgBySlug resolves an organization by its URL slug.
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

// GetByID fetches a single undeleted notification scoped to org + user.
func (r *Repository) GetByID(ctx context.Context, id, orgID, userID pgtype.UUID) (*db.Notification, error) {
	n, err := r.queries.GetNotificationByID(ctx, db.GetNotificationByIDParams{
		ID:             id,
		OrganizationID: orgID,
		UserID:         userID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotificationNotFound
		}
		return nil, err
	}
	return &n, nil
}

// List returns a paginated page of notifications for a user within an org.
func (r *Repository) List(ctx context.Context, orgID, userID pgtype.UUID, params ListParams) ([]db.Notification, error) {
	return r.queries.ListNotificationsByUser(ctx, db.ListNotificationsByUserParams{
		OrganizationID: orgID,
		UserID:         userID,
		PageLimit:      params.Limit,
		PageOffset:     params.Offset,
	})
}

// Count returns the total count for pagination metadata.
func (r *Repository) Count(ctx context.Context, orgID, userID pgtype.UUID) (int64, error) {
	return r.queries.CountNotificationsByUser(ctx, db.CountNotificationsByUserParams{
		OrganizationID: orgID,
		UserID:         userID,
	})
}

// GetPreferences returns all preference rows for a user within an org.
func (r *Repository) GetPreferences(ctx context.Context, orgID, userID pgtype.UUID) ([]db.NotificationPreference, error) {
	return r.queries.GetUserPreferences(ctx, db.GetUserPreferencesParams{
		OrganizationID: orgID,
		UserID:         userID,
	})
}

// GetPreference fetches a single preference; returns ErrPreferenceNotFound if absent.
func (r *Repository) GetPreference(ctx context.Context, orgID, userID pgtype.UUID, eventType db.NotificationEventType, channel db.NotificationChannel) (*db.NotificationPreference, error) {
	p, err := r.queries.GetUserPreference(ctx, db.GetUserPreferenceParams{
		OrganizationID: orgID,
		UserID:         userID,
		EventType:      eventType,
		Channel:        channel,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrPreferenceNotFound
		}
		return nil, err
	}
	return &p, nil
}

// ── transactional writes ──────────────────────────────────────────────────────

// MarkReadWithAudit sets read_at on a single notification.
// No audit record is written for read operations per spec.
func (r *Repository) MarkRead(ctx context.Context, id, orgID, userID pgtype.UUID) (*db.Notification, error) {
	n, err := r.queries.MarkNotificationRead(ctx, db.MarkNotificationReadParams{
		ID:             id,
		OrganizationID: orgID,
		UserID:         userID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			// Either already read or not found; treat as not found.
			return nil, ErrNotificationNotFound
		}
		return nil, err
	}
	return &n, nil
}

// MarkAllRead marks all unread notifications for a user as read.
// No audit record per spec.
func (r *Repository) MarkAllRead(ctx context.Context, orgID, userID pgtype.UUID) error {
	return r.queries.MarkAllNotificationsRead(ctx, db.MarkAllNotificationsReadParams{
		OrganizationID: orgID,
		UserID:         userID,
	})
}

// SoftDelete sets deleted_at on a notification. Returns ErrNotificationNotFound
// if the notification does not exist or is already deleted.
// No audit record per spec.
func (r *Repository) SoftDelete(ctx context.Context, id, orgID, userID pgtype.UUID) error {
	n, err := r.queries.SoftDeleteNotification(ctx, db.SoftDeleteNotificationParams{
		ID:             id,
		OrganizationID: orgID,
		UserID:         userID,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotificationNotFound
	}
	return nil
}

// UpsertPreference creates or updates a preference (last-writer-wins).
// Writes an audit record (AuditActionUpdate) with old and new data per spec.
func (r *Repository) UpsertPreference(
	ctx context.Context,
	orgID, userID pgtype.UUID,
	eventType db.NotificationEventType,
	channel db.NotificationChannel,
	enabled bool,
	actorID pgtype.UUID,
) (*db.NotificationPreference, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	// Capture old state for audit. ErrNoRows means this is a first-time creation.
	// isCreate drives the audit action below:
	//   true  → AuditActionCreate (old_data NULL is valid for create)
	//   false → AuditActionUpdate (both old_data and new_data are required by
	//            chk_audit_update_has_both_snapshots)
	var oldData []byte
	isCreate := false
	old, err := qtx.GetUserPreference(ctx, db.GetUserPreferenceParams{
		OrganizationID: orgID,
		UserID:         userID,
		EventType:      eventType,
		Channel:        channel,
	})
	if err == nil {
		oldData, _ = prefToAuditJSON(&old)
	} else if err == pgx.ErrNoRows {
		isCreate = true
	} else {
		return nil, err
	}

	pref, err := qtx.UpsertNotificationPreference(ctx, db.UpsertNotificationPreferenceParams{
		OrganizationID: orgID,
		UserID:         userID,
		EventType:      eventType,
		Channel:        channel,
		Enabled:        enabled,
	})
	if err != nil {
		return nil, err
	}

	newData, err := prefToAuditJSON(&pref)
	if err != nil {
		return nil, err
	}

	// Select audit action based on whether the row is new or existing.
	// AuditActionCreate: old_data may be NULL (chk_audit_update_has_both_snapshots
	//   only constrains 'update' actions — schema allows NULL old_data for 'create').
	// AuditActionUpdate: both old_data and new_data are required by the constraint.
	auditAction := db.AuditActionUpdate
	if isCreate {
		auditAction = db.AuditActionCreate
	}

	if err := qtx.CreateAuditLog(ctx, db.CreateAuditLogParams{
		OrganizationID: orgID,
		UserID:         actorID,
		Action:         auditAction,
		EntityType:     "notification_preferences",
		EntityID:       pref.ID,
		OldData:        oldData,
		NewData:        newData,
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &pref, nil
}

// ── DrainOutbox ───────────────────────────────────────────────────────────────

// prefKey is the composite key used to deduplicate preference batch loads.
// Both fields are string-typed ENUMs and are therefore comparable map keys.
type prefKey struct {
	eventType db.NotificationEventType
	channel   db.NotificationChannel
}

// drainChannels lists the channels DrainOutbox fans out to synchronously.
var drainChannels = []db.NotificationChannel{
	db.NotificationChannelInApp,
	db.NotificationChannelEmail,
}

// DrainOutbox claims pending outbox entries for the organization using
// FOR UPDATE SKIP LOCKED, fans out in_app and email notifications to all
// org members (filtered per-channel by preferences), and marks each entry
// processed.
//
// Returns all newly-created in_app notification rows so the caller can publish
// SSE events after the transaction commits (publish-after-commit).
//
// sent_at semantics:
//
//	in_app: set to NOW() — delivered synchronously via the inbox API.
//	email:  left NULL   — pending async delivery by EmailWorker.
//
// Called synchronously by domain services after their transaction commits.
// Safe to retry: ON CONFLICT (outbox_id, user_id, channel) DO NOTHING in the
// INSERT makes every drain idempotent; pgx returns pgx.ErrNoRows on conflict,
// which DrainOutbox treats as "already drained" and skips.
//
// Preference resolution complexity:
//
//	O(unique event-type/channel pairs) SQL queries — typically O(2).
func (r *Repository) DrainOutbox(ctx context.Context, orgID pgtype.UUID) ([]db.Notification, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := r.queries.WithTx(tx)

	// Claim pending entries (SKIP LOCKED: concurrent drains on same org skip).
	entries, err := qtx.DrainOutboxEntries(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, tx.Commit(ctx)
	}

	// Get all current org members for fan-out.
	// members is []pgtype.UUID — one element per distinct user with a role in the org.
	members, err := qtx.GetOrgMembersForNotification(ctx, orgID)
	if err != nil {
		return nil, err
	}

	// ── Batch preference loading ───────────────────────────────────────────────
	// For each unique (event_type, channel) pair across all pending entries,
	// issue one GetNotificationPreferencesForEvent query.
	// Build an in-memory lookup: prefKey → (userID bytes → enabled).
	//
	// A user absent from the result set has no preference row, treated as
	// enabled = TRUE (default opt-out model). Only opted-out users appear
	// in the map with enabled = false.
	prefCache := make(map[prefKey]map[[16]byte]bool)

	for _, entry := range entries {
		for _, ch := range drainChannels {
			key := prefKey{entry.EventType, ch}
			if _, loaded := prefCache[key]; loaded {
				continue
			}
			rows, err := qtx.GetNotificationPreferencesForEvent(ctx, db.GetNotificationPreferencesForEventParams{
				OrganizationID: orgID,
				EventType:      entry.EventType,
				Channel:        ch,
			})
			if err != nil {
				return nil, err
			}
			userMap := make(map[[16]byte]bool, len(rows))
			for _, row := range rows {
				userMap[row.UserID.Bytes] = row.Enabled
			}
			prefCache[key] = userMap
		}
	}
	// ── end batch preference loading ───────────────────────────────────────────

	var created []db.Notification

	for _, entry := range entries {
		for _, memberUID := range members {
			// Skip the actor — they triggered the event and don't need a notification.
			if entry.ActorID.Valid && entry.ActorID.Bytes == memberUID.Bytes {
				continue
			}

			for _, ch := range drainChannels {
				key := prefKey{entry.EventType, ch}
				userPrefs := prefCache[key]

				// Resolve per-channel preference (no SQL round-trip).
				// Missing entry → no preference row → enabled by default.
				shouldNotify := true
				if enabled, exists := userPrefs[memberUID.Bytes]; exists {
					shouldNotify = enabled
				}
				if !shouldNotify {
					continue
				}

				// in_app: sent_at = NOW() (delivered synchronously via inbox API).
				// email:  sent_at = NULL (pending async delivery by EmailWorker).
				var sentAt pgtype.Timestamptz
				if ch == db.NotificationChannelInApp {
					sentAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
				}

				n, insertErr := qtx.CreateNotification(ctx, db.CreateNotificationParams{
					OrganizationID: entry.OrganizationID,
					UserID:         memberUID,
					OutboxID:       entry.ID,
					Channel:        ch,
					EventType:      entry.EventType,
					EntityType:     entry.EntityType,
					EntityID:       entry.EntityID,
					Payload:        entry.Payload,
					SentAt:         sentAt,
				})
				if insertErr != nil {
					// ON CONFLICT DO NOTHING → already drained (idempotent path).
					if insertErr == pgx.ErrNoRows {
						continue
					}
					return nil, insertErr
				}
				// Collect newly-created in_app rows for SSE publishing after commit.
				if ch == db.NotificationChannelInApp {
					created = append(created, n)
				}
			}
		}

		// ── webhook fan-out ───────────────────────────────────────────────────
		// Insert one webhook_deliveries row per active endpoint in the org.
		// ON CONFLICT DO NOTHING in CreateWebhookDelivery makes retries idempotent.
		endpoints, err := qtx.GetActiveWebhookEndpointsForOrg(ctx, orgID)
		if err != nil {
			return nil, err
		}
		for _, ep := range endpoints {
			_, createErr := qtx.CreateWebhookDelivery(ctx, db.CreateWebhookDeliveryParams{
				OrganizationID: entry.OrganizationID,
				EndpointID:     ep.ID,
				OutboxID:       entry.ID,
				EventType:      entry.EventType,
				EntityType:     entry.EntityType,
				EntityID:       entry.EntityID,
				Payload:        entry.Payload,
			})
			// ON CONFLICT DO NOTHING returns pgx.ErrNoRows when the row already exists.
			// This is the idempotency path — not an error.
			if createErr != nil && createErr != pgx.ErrNoRows {
				return nil, createErr
			}
		}
		// ── end webhook fan-out ───────────────────────────────────────────────

		// Mark outbox entry processed.
		if err := qtx.MarkOutboxEntryProcessed(ctx, entry.ID); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return created, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func prefToAuditJSON(p *db.NotificationPreference) ([]byte, error) {
	return json.Marshal(map[string]any{
		"id":              pgutil.UUIDToString(p.ID),
		"organization_id": pgutil.UUIDToString(p.OrganizationID),
		"user_id":         pgutil.UUIDToString(p.UserID),
		"event_type":      string(p.EventType),
		"channel":         string(p.Channel),
		"enabled":         p.Enabled,
		"updated_at":      p.UpdatedAt.Time.UTC().Format(time.RFC3339),
	})
}
