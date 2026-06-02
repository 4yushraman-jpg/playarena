package notifications

import "encoding/json"

// ─── Notification responses ────────────────────────────────────────────────────

// Response is the JSON shape returned for a single notification.
type Response struct {
	ID             string          `json:"id"`
	OrganizationID string          `json:"organization_id"`
	UserID         string          `json:"user_id"`
	OutboxID       string          `json:"outbox_id"`
	Channel        string          `json:"channel"`
	EventType      string          `json:"event_type"`
	EntityType     string          `json:"entity_type"`
	EntityID       string          `json:"entity_id"`
	Payload        json.RawMessage `json:"payload"`
	ReadAt         *string         `json:"read_at"`
	SentAt         *string         `json:"sent_at"`
	CreatedAt      string          `json:"created_at"`
}

// ListResponse wraps a paginated slice of notifications.
type ListResponse struct {
	Notifications []Response `json:"notifications"`
	Total         int64      `json:"total"`
	Limit         int        `json:"limit"`
	Offset        int        `json:"offset"`
}

// ─── Preference responses ──────────────────────────────────────────────────────

// PreferenceResponse is the JSON shape for a single notification preference.
type PreferenceResponse struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	UserID         string `json:"user_id"`
	EventType      string `json:"event_type"`
	Channel        string `json:"channel"`
	Enabled        bool   `json:"enabled"`
	UpdatedAt      string `json:"updated_at"`
}

// PreferencesListResponse wraps all preferences for a user in an org.
type PreferencesListResponse struct {
	Preferences []PreferenceResponse `json:"preferences"`
}

// ─── Preference request ────────────────────────────────────────────────────────

// UpdatePreferenceRequest is the body for PUT /preferences/{event_type}.
type UpdatePreferenceRequest struct {
	Channel string `json:"channel" validate:"required"`
	Enabled bool   `json:"enabled"`
}
