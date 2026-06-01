package matches

// ── request DTOs ──────────────────────────────────────────────────────────────

// CreateRequest is the payload for
// POST /api/v1/organizations/{slug}/matches.
//
// tournament_id and scheduled_at are required.
// Participant fields are validated against the tournament's participant_type:
//   - team tournament:       home_team_id + away_team_id required; player IDs forbidden
//   - individual tournament: home_player_id + away_player_id required; team IDs forbidden
type CreateRequest struct {
	TournamentID string  `json:"tournament_id" validate:"required,uuid"`
	RoundNumber  *int16  `json:"round_number"`
	RoundName    *string `json:"round_name"`
	MatchNumber  *int16  `json:"match_number"`
	HomeTeamID   *string `json:"home_team_id"`
	AwayTeamID   *string `json:"away_team_id"`
	HomePlayerID *string `json:"home_player_id"`
	AwayPlayerID *string `json:"away_player_id"`
	Venue        *string `json:"venue"`
	ScheduledAt  string  `json:"scheduled_at" validate:"required"`
	Notes        *string `json:"notes"`
}

// UpdateRequest is the payload for
// PATCH /api/v1/organizations/{slug}/matches/{id}.
// All fields are optional; omitting a field preserves the current value.
//
// Status transitions (Phase 8A):
//
//	scheduled → live | cancelled
//	live      → completed | abandoned | cancelled
//	completed, cancelled, abandoned → (terminal; no transitions)
//
// Timestamp rules:
//   - Transitioning to live stamps started_at automatically.
//   - Transitioning to completed or abandoned stamps ended_at automatically.
//
// Winner rules:
//   - winner_team_id / winner_player_id may only be set when status == completed.
//   - Winner must equal home or away participant.
type UpdateRequest struct {
	RoundNumber    *int16  `json:"round_number"`
	RoundName      *string `json:"round_name"`
	MatchNumber    *int16  `json:"match_number"`
	HomeTeamID     *string `json:"home_team_id"`
	AwayTeamID     *string `json:"away_team_id"`
	HomePlayerID   *string `json:"home_player_id"`
	AwayPlayerID   *string `json:"away_player_id"`
	Venue          *string `json:"venue"`
	ScheduledAt    *string `json:"scheduled_at"`
	Status         *string `json:"status"`
	WinnerTeamID   *string `json:"winner_team_id"`
	WinnerPlayerID *string `json:"winner_player_id"`
	Notes          *string `json:"notes"`
}

// ── response DTOs ─────────────────────────────────────────────────────────────

// Response is the unified match representation returned by all endpoints.
type Response struct {
	ID             string  `json:"id"`
	TournamentID   string  `json:"tournament_id"`
	OrganizationID string  `json:"organization_id"`
	RoundNumber    *int16  `json:"round_number,omitempty"`
	RoundName      *string `json:"round_name,omitempty"`
	MatchNumber    *int16  `json:"match_number,omitempty"`
	HomeTeamID     *string `json:"home_team_id,omitempty"`
	AwayTeamID     *string `json:"away_team_id,omitempty"`
	HomePlayerID   *string `json:"home_player_id,omitempty"`
	AwayPlayerID   *string `json:"away_player_id,omitempty"`
	Venue          *string `json:"venue,omitempty"`
	ScheduledAt    *string `json:"scheduled_at,omitempty"`
	StartedAt      *string `json:"started_at,omitempty"`
	EndedAt        *string `json:"ended_at,omitempty"`
	Status         string  `json:"status"`
	WinnerTeamID   *string `json:"winner_team_id,omitempty"`
	WinnerPlayerID *string `json:"winner_player_id,omitempty"`
	IsWalkover     bool    `json:"is_walkover"`
	// HomeScore and AwayScore are the final snapshotted scores.
	// Both are 0 for any non-completed match and for walkovers.
	HomeScore int32   `json:"home_score"`
	AwayScore int32   `json:"away_score"`
	Notes     *string `json:"notes,omitempty"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

// ListResponse wraps a paginated list of matches.
type ListResponse struct {
	Matches []Response `json:"matches"`
	Total   int64      `json:"total"`
	Limit   int        `json:"limit"`
	Offset  int        `json:"offset"`
}
