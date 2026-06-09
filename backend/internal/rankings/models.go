package rankings

const (
	DefaultListLimit int32 = 50
	MaxListLimit     int32 = 200
)

// StatsRow carries one participant's final standings for a completed tournament.
// Used as input to SnapshotTeamStats and SnapshotPlayerStats.
type StatsRow struct {
	ParticipantID string
	Position      int
	Played        int
	Wins          int
	Draws         int
	Losses        int
	Points        int
	ScoreFor      int
	ScoreAgainst  int
}

// ListParams carries pagination parameters for rankings queries.
type ListParams struct {
	Limit  int32
	Offset int32
}

// PlayerRankingEntry is one row in the player rankings list response.
type PlayerRankingEntry struct {
	Rank              int     `json:"rank"`
	PlayerID          string  `json:"player_id"`
	DisplayName       string  `json:"display_name"`
	TournamentsPlayed int     `json:"tournaments_played"`
	TournamentsWon    int     `json:"tournaments_won"`
	PodiumFinishes    int     `json:"podium_finishes"`
	TotalMatches      int     `json:"total_matches"`
	TotalWins         int     `json:"total_wins"`
	TotalDraws        int     `json:"total_draws"`
	TotalLosses       int     `json:"total_losses"`
	TotalPoints       int     `json:"total_points"`
	WinRate           float64 `json:"win_rate"`
	ScoreFor          int     `json:"score_for"`
	ScoreAgainst      int     `json:"score_against"`
}

// PlayerRankingsResponse is the full paginated response for player rankings.
type PlayerRankingsResponse struct {
	OrganizationID string               `json:"organization_id"`
	Rankings       []PlayerRankingEntry `json:"rankings"`
	Total          int64                `json:"total"`
	Limit          int                  `json:"limit"`
	Offset         int                  `json:"offset"`
}

// TeamRankingEntry is one row in the team rankings list response.
type TeamRankingEntry struct {
	Rank              int     `json:"rank"`
	TeamID            string  `json:"team_id"`
	TeamName          string  `json:"team_name"`
	TournamentsPlayed int     `json:"tournaments_played"`
	TournamentsWon    int     `json:"tournaments_won"`
	PodiumFinishes    int     `json:"podium_finishes"`
	TotalMatches      int     `json:"total_matches"`
	TotalWins         int     `json:"total_wins"`
	TotalDraws        int     `json:"total_draws"`
	TotalLosses       int     `json:"total_losses"`
	TotalPoints       int     `json:"total_points"`
	WinRate           float64 `json:"win_rate"`
	ScoreFor          int     `json:"score_for"`
	ScoreAgainst      int     `json:"score_against"`
}

// TeamRankingsResponse is the full paginated response for team rankings.
type TeamRankingsResponse struct {
	OrganizationID string             `json:"organization_id"`
	Rankings       []TeamRankingEntry `json:"rankings"`
	Total          int64              `json:"total"`
	Limit          int                `json:"limit"`
	Offset         int                `json:"offset"`
}
