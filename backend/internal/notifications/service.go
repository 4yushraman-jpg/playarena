package notifications

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/realtime"
)

// Service implements the notifications use-cases.
type Service struct {
	repo *Repository
	hub  *realtime.Hub
	log  *slog.Logger
}

// NewService constructs a Service. hub may be nil (SSE publishing is a no-op).
func NewService(repo *Repository, hub *realtime.Hub, log *slog.Logger) *Service {
	return &Service{repo: repo, hub: hub, log: log}
}

// ── public API methods ────────────────────────────────────────────────────────

// List returns a paginated list of notifications for the authenticated user
// within the org identified by orgSlug.
func (s *Service) List(ctx context.Context, orgSlug, userID string, params ListParams) (*ListResponse, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	uid, err := pgutil.ParseUUID(userID)
	if err != nil {
		return nil, ErrForbidden
	}

	if params.Limit <= 0 || params.Limit > MaxListLimit {
		params.Limit = DefaultListLimit
	}
	if params.Offset < 0 {
		params.Offset = 0
	}

	ns, err := s.repo.List(ctx, org.ID, uid, params)
	if err != nil {
		return nil, err
	}

	total, err := s.repo.Count(ctx, org.ID, uid)
	if err != nil {
		return nil, err
	}

	resp := make([]Response, len(ns))
	for i := range ns {
		resp[i] = notificationToResponse(&ns[i])
	}
	return &ListResponse{
		Notifications: resp,
		Total:         total,
		Limit:         int(params.Limit),
		Offset:        int(params.Offset),
	}, nil
}

// GetByID retrieves a single notification scoped to the user and org.
func (s *Service) GetByID(ctx context.Context, orgSlug, notifID, userID string) (*Response, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	nid, err := pgutil.ParseUUID(notifID)
	if err != nil {
		return nil, ErrNotificationNotFound
	}

	uid, err := pgutil.ParseUUID(userID)
	if err != nil {
		return nil, ErrForbidden
	}

	n, err := s.repo.GetByID(ctx, nid, org.ID, uid)
	if err != nil {
		return nil, err
	}
	r := notificationToResponse(n)
	return &r, nil
}

// MarkRead marks a single notification as read.
func (s *Service) MarkRead(ctx context.Context, orgSlug, notifID, userID string) (*Response, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	nid, err := pgutil.ParseUUID(notifID)
	if err != nil {
		return nil, ErrNotificationNotFound
	}

	uid, err := pgutil.ParseUUID(userID)
	if err != nil {
		return nil, ErrForbidden
	}

	n, err := s.repo.MarkRead(ctx, nid, org.ID, uid)
	if err != nil {
		return nil, err
	}
	r := notificationToResponse(n)
	return &r, nil
}

// MarkAllRead marks all unread notifications for the user within the org as read.
func (s *Service) MarkAllRead(ctx context.Context, orgSlug, userID string) error {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return err
	}

	uid, err := pgutil.ParseUUID(userID)
	if err != nil {
		return ErrForbidden
	}

	return s.repo.MarkAllRead(ctx, org.ID, uid)
}

// Delete soft-deletes a notification for the user.
func (s *Service) Delete(ctx context.Context, orgSlug, notifID, userID string) error {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return err
	}

	nid, err := pgutil.ParseUUID(notifID)
	if err != nil {
		return ErrNotificationNotFound
	}

	uid, err := pgutil.ParseUUID(userID)
	if err != nil {
		return ErrForbidden
	}

	return s.repo.SoftDelete(ctx, nid, org.ID, uid)
}

// ── preferences ───────────────────────────────────────────────────────────────

// GetPreferences returns all stored preferences for the user within the org.
// A missing preference means the channel/event_type combination is enabled by default.
func (s *Service) GetPreferences(ctx context.Context, orgSlug, userID string) (*PreferencesListResponse, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	uid, err := pgutil.ParseUUID(userID)
	if err != nil {
		return nil, ErrForbidden
	}

	prefs, err := s.repo.GetPreferences(ctx, org.ID, uid)
	if err != nil {
		return nil, err
	}

	resp := make([]PreferenceResponse, len(prefs))
	for i := range prefs {
		resp[i] = preferenceToResponse(&prefs[i])
	}
	return &PreferencesListResponse{Preferences: resp}, nil
}

// UpdatePreference upserts a preference (last-writer-wins).
// Preference changes are audited with old_data + new_data per spec.
func (s *Service) UpdatePreference(
	ctx context.Context,
	orgSlug, eventTypeStr, userID, actorID string,
	req UpdatePreferenceRequest,
) (*PreferenceResponse, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	uid, err := pgutil.ParseUUID(userID)
	if err != nil {
		return nil, ErrForbidden
	}

	actorUID, err := pgutil.ParseUUID(actorID)
	if err != nil {
		return nil, ErrForbidden
	}

	eventType, err := parseEventType(eventTypeStr)
	if err != nil {
		return nil, err
	}

	channel, err := parseChannel(req.Channel)
	if err != nil {
		return nil, err
	}

	pref, err := s.repo.UpsertPreference(ctx, org.ID, uid, eventType, channel, req.Enabled, actorUID)
	if err != nil {
		return nil, err
	}

	r := preferenceToResponse(pref)
	return &r, nil
}

// ── DrainOutbox (called by domain services post-commit) ───────────────────────

// DrainOutbox fans out pending outbox entries for the org into in_app and email
// notifications, then publishes SSE events for newly-created in_app rows.
// Called synchronously by domain services immediately after their commit.
// Errors are logged but not propagated: drain failure must not fail the domain
// operation that already committed. SSE publish is best-effort (at-most-once).
func (s *Service) DrainOutbox(ctx context.Context, orgID pgtype.UUID, log *slog.Logger) {
	created, err := s.repo.DrainOutbox(ctx, orgID)
	if err != nil {
		log.Error("notifications: DrainOutbox failed",
			slog.String("org_id", pgutil.UUIDToString(orgID)),
			slog.Any("error", err),
		)
		return
	}
	if s.hub == nil || len(created) == 0 {
		return
	}
	for i := range created {
		n := &created[i]
		s.hub.Publish(n.OrganizationID, n.UserID, notificationToResponse(n))
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func notificationToResponse(n *db.Notification) Response {
	var payload json.RawMessage
	if len(n.Payload) > 0 {
		payload = json.RawMessage(n.Payload)
	} else {
		payload = json.RawMessage("{}")
	}

	return Response{
		ID:             pgutil.UUIDToString(n.ID),
		OrganizationID: pgutil.UUIDToString(n.OrganizationID),
		UserID:         pgutil.UUIDToString(n.UserID),
		OutboxID:       pgutil.UUIDToString(n.OutboxID),
		Channel:        string(n.Channel),
		EventType:      string(n.EventType),
		EntityType:     n.EntityType,
		EntityID:       pgutil.UUIDToString(n.EntityID),
		Payload:        payload,
		ReadAt:         tsPtr(n.ReadAt),
		SentAt:         tsPtr(n.SentAt),
		CreatedAt:      n.CreatedAt.Time.UTC().Format(time.RFC3339),
	}
}

func preferenceToResponse(p *db.NotificationPreference) PreferenceResponse {
	return PreferenceResponse{
		ID:             pgutil.UUIDToString(p.ID),
		OrganizationID: pgutil.UUIDToString(p.OrganizationID),
		UserID:         pgutil.UUIDToString(p.UserID),
		EventType:      string(p.EventType),
		Channel:        string(p.Channel),
		Enabled:        p.Enabled,
		UpdatedAt:      p.UpdatedAt.Time.UTC().Format(time.RFC3339),
	}
}

func tsPtr(ts pgtype.Timestamptz) *string {
	if !ts.Valid {
		return nil
	}
	s := ts.Time.UTC().Format(time.RFC3339)
	return &s
}

func parseEventType(s string) (db.NotificationEventType, error) {
	et := db.NotificationEventType(strings.ToLower(strings.TrimSpace(s)))
	switch et {
	case db.NotificationEventTypeMatchCreated,
		db.NotificationEventTypeMatchStarted,
		db.NotificationEventTypeMatchCompleted,
		db.NotificationEventTypeMatchCancelled,
		db.NotificationEventTypeMatchAbandoned,
		db.NotificationEventTypeTournamentStatusChanged,
		db.NotificationEventTypeRegistrationApproved,
		db.NotificationEventTypeRegistrationRejected,
		db.NotificationEventTypeRegistrationWithdrawn:
		return et, nil
	}
	return "", ErrInvalidEventType
}

func parseChannel(s string) (db.NotificationChannel, error) {
	ch := db.NotificationChannel(strings.ToLower(strings.TrimSpace(s)))
	switch ch {
	case db.NotificationChannelInApp,
		db.NotificationChannelEmail,
		db.NotificationChannelWebhook:
		return ch, nil
	}
	return "", ErrInvalidChannel
}
