package scoring

// ScoreResult is the computed score for a match derived from the effective
// event log.  It is never stored — the event log is the source of truth and
// this value is always recomputable.
//
// HomeTeamID / AwayTeamID are populated for team-format tournaments.
// HomePlayerID / AwayPlayerID are populated for individual-format tournaments.
// The omitempty tags ensure unused fields are omitted from the JSON response.
type ScoreResult struct {
	MatchID      string `json:"match_id"`
	MatchStatus  string `json:"match_status"`
	IsWalkover   bool   `json:"is_walkover"`
	HomeTeamID   string `json:"home_team_id,omitempty"`
	AwayTeamID   string `json:"away_team_id,omitempty"`
	HomePlayerID string `json:"home_player_id,omitempty"`
	AwayPlayerID string `json:"away_player_id,omitempty"`
	HomeScore    int    `json:"home_score"`
	AwayScore    int    `json:"away_score"`
}
