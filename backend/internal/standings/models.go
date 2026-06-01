// Package standings is a pure computation package — no DB access, no HTTP
// types, no side effects.  Its sole input is completed-match data and
// registration metadata already fetched by the tournaments service.
package standings

import "time"

// CompletedMatch is the minimal snapshot of a completed match used for
// standings derivation.  All values come from the matches table — the
// standings engine never reads match_events.
type CompletedMatch struct {
	HomeParticipantID string // team or player UUID
	AwayParticipantID string
	HomeScore         int
	AwayScore         int
	// WinnerID is the participant UUID of the winner.  Empty string means draw
	// (completed match with no winner set).
	WinnerID   string
	IsWalkover bool
}

// RegistrationInfo carries tiebreaker metadata for a single approved
// tournament registrant.
type RegistrationInfo struct {
	ParticipantID string
	SeedNumber    *int16    // nil when the organiser has not assigned a seed
	RegisteredAt  time.Time // always present; used as final deterministic tiebreaker
}

// Settings holds the point values used for standings computation.
// Values are sourced from tournaments.settings JSONB; defaults apply when a
// field is absent.  See DefaultSettings for the baseline values.
type Settings struct {
	WinPoints  int // default 3
	DrawPoints int // default 1
	LossPoints int // default 0

	// CloseMargin, when > 0, enables the close-loss rule: a team that loses
	// by ≤ CloseMargin points receives CloseLossPoints instead of LossPoints.
	// Walkovers are always exempt from this rule.
	CloseMargin     int // default 0 (disabled)
	CloseLossPoints int // default 0
}

// DefaultSettings returns the point system used when tournaments.settings
// does not provide overrides.
func DefaultSettings() Settings {
	return Settings{WinPoints: 3, DrawPoints: 1, LossPoints: 0}
}

// StandingsRow is one participant's record in the ordered standings table.
type StandingsRow struct {
	ParticipantID   string
	Position        int
	Played          int
	Wins            int
	Losses          int
	Draws           int
	Points          int
	ScoreFor        int
	ScoreAgainst    int
	ScoreDifference int // ScoreFor - ScoreAgainst
	SeedNumber      *int16
	RegisteredAt    time.Time
}
