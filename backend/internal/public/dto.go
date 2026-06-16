package public

// Public DTOs (PRI-1). These define the ENTIRE shape of data exposed to the
// anonymous world. No field here may carry PII, audit, generation metadata,
// created_by/updated_by, internal notes, or registration/membership data.

// OrganizationRef is the minimal public org branding.
type OrganizationRef struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// PublicTournament is the overview payload.
type PublicTournament struct {
	Name                 string          `json:"name"`
	Slug                 string          `json:"slug"`
	Sport                string          `json:"sport"`
	Format               string          `json:"format"`
	ParticipantType      string          `json:"participant_type"`
	Status               string          `json:"status"`
	Visibility           string          `json:"visibility"`
	BannerURL            *string         `json:"banner_url,omitempty"`
	Description          *string         `json:"description,omitempty"`
	PrizePool            *string         `json:"prize_pool,omitempty"`
	Currency             string          `json:"currency"`
	MaxParticipants      *int16          `json:"max_participants,omitempty"`
	RegistrationOpensAt  *string         `json:"registration_opens_at,omitempty"`
	RegistrationClosesAt *string         `json:"registration_closes_at,omitempty"`
	StartsAt             *string         `json:"starts_at,omitempty"`
	EndsAt               *string         `json:"ends_at,omitempty"`
	Venue                *string         `json:"venue,omitempty"`
	City                 *string         `json:"city,omitempty"`
	Country              *string         `json:"country,omitempty"`
	Rules                *string         `json:"rules,omitempty"`
	Organization         OrganizationRef `json:"organization"`
}

// PublicSlot is one side of a public match: a resolved name, or TBD/qualifier.
type PublicSlot struct {
	// Kind: "participant" | "tbd" | "qualifier".
	Kind string `json:"kind"`
	// Name is the resolved team/player display name (participant), the qualifier
	// label (e.g. "Group A #1"), or "TBD".
	Name string `json:"name"`
}

// PublicMatch is one whitelisted fixture/result row.
type PublicMatch struct {
	ID          string     `json:"id"`
	RoundNumber *int16     `json:"round_number,omitempty"`
	RoundName   *string    `json:"round_name,omitempty"`
	MatchNumber *int16     `json:"match_number,omitempty"`
	GroupLabel  *string    `json:"group_label,omitempty"`
	Home        PublicSlot `json:"home"`
	Away        PublicSlot `json:"away"`
	ScheduledAt *string    `json:"scheduled_at,omitempty"`
	Venue       *string    `json:"venue,omitempty"`
	Status      string     `json:"status"`
	HomeScore   int32      `json:"home_score"`
	AwayScore   int32      `json:"away_score"`
	IsWalkover  bool       `json:"is_walkover"`
	// WinnerName is the resolved winner display name, empty for draw/undecided.
	WinnerName    string  `json:"winner_name,omitempty"`
	NextMatchID   *string `json:"next_match_id,omitempty"`
	NextMatchSlot *int16  `json:"next_match_slot,omitempty"`
}

// PublicMatchesResponse wraps the fixture list.
type PublicMatchesResponse struct {
	Matches []PublicMatch `json:"matches"`
}

// PublicStandingsRow is one public standings entry.
type PublicStandingsRow struct {
	Position        int    `json:"position"`
	ParticipantName string `json:"participant_name"`
	Played          int    `json:"played"`
	Wins            int    `json:"wins"`
	Losses          int    `json:"losses"`
	Draws           int    `json:"draws"`
	Points          int    `json:"points"`
	ScoreFor        int    `json:"score_for"`
	ScoreAgainst    int    `json:"score_against"`
	ScoreDifference int    `json:"score_difference"`
	Disqualified    bool   `json:"disqualified"`
}

// PublicStandingsResponse is the standings table + the point system in use.
type PublicStandingsResponse struct {
	Format      string               `json:"format"`
	Status      string               `json:"status"`
	PointSystem PublicPointSystem    `json:"point_system"`
	Standings   []PublicStandingsRow `json:"standings"`
}

// PublicPointSystem mirrors the organizer-facing point system display.
type PublicPointSystem struct {
	WinPoints       int `json:"win_points"`
	DrawPoints      int `json:"draw_points"`
	LossPoints      int `json:"loss_points"`
	CloseMargin     int `json:"close_margin,omitempty"`
	CloseLossPoints int `json:"close_loss_points,omitempty"`
}

// PublicMatchEvent is one whitelisted timeline event.
type PublicMatchEvent struct {
	Sequence     int64   `json:"sequence"`
	EventType    string  `json:"event_type"`
	Period       *int16  `json:"period,omitempty"`
	ClockSeconds *int32  `json:"clock_seconds,omitempty"`
	ActorName    string  `json:"actor_name,omitempty"` // resolved team/player name
	RecordedAt   *string `json:"recorded_at,omitempty"`
}

// PublicMatchDetail is the single-match view for spectators following one game.
type PublicMatchDetail struct {
	ID          string             `json:"id"`
	RoundName   *string            `json:"round_name,omitempty"`
	GroupLabel  *string            `json:"group_label,omitempty"`
	Home        PublicSlot         `json:"home"`
	Away        PublicSlot         `json:"away"`
	ScheduledAt *string            `json:"scheduled_at,omitempty"`
	StartedAt   *string            `json:"started_at,omitempty"`
	EndedAt     *string            `json:"ended_at,omitempty"`
	Venue       *string            `json:"venue,omitempty"`
	Status      string             `json:"status"`
	HomeScore   int32              `json:"home_score"`
	AwayScore   int32              `json:"away_score"`
	IsWalkover  bool               `json:"is_walkover"`
	WinnerName  string             `json:"winner_name,omitempty"`
	Timeline    []PublicMatchEvent `json:"timeline"`
}
