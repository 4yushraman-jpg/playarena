package rankings

import (
	"context"

	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
)

// Service implements rankings use-cases.
type Service struct {
	repo *Repository
}

// NewService constructs a Service.
func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// ListPlayerRankings returns paginated all-time player rankings for an org.
// No ownership check: any authenticated user may read rankings.
func (s *Service) ListPlayerRankings(ctx context.Context, orgSlug string, params ListParams) (*PlayerRankingsResponse, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	if params.Limit <= 0 || params.Limit > MaxListLimit {
		params.Limit = DefaultListLimit
	}
	if params.Offset < 0 {
		params.Offset = 0
	}

	rows, err := s.repo.ListPlayerRankings(ctx, org.ID, params)
	if err != nil {
		return nil, err
	}

	total, err := s.repo.CountPlayerRankings(ctx, org.ID)
	if err != nil {
		return nil, err
	}

	entries := make([]PlayerRankingEntry, len(rows))
	for i, r := range rows {
		entries[i] = PlayerRankingEntry{
			Rank:              int(r.Rank),
			PlayerID:          pgutil.UUIDToString(r.PlayerID),
			DisplayName:       r.DisplayName,
			TournamentsPlayed: int(r.TournamentsPlayed),
			TournamentsWon:    int(r.TournamentsWon),
			PodiumFinishes:    int(r.PodiumFinishes),
			TotalMatches:      int(r.TotalMatches),
			TotalWins:         int(r.TotalWins),
			TotalDraws:        int(r.TotalDraws),
			TotalLosses:       int(r.TotalLosses),
			TotalPoints:       int(r.TotalPoints),
			WinRate:           winRate(int(r.TotalWins), int(r.TotalMatches)),
			ScoreFor:          int(r.ScoreFor),
			ScoreAgainst:      int(r.ScoreAgainst),
		}
	}

	return &PlayerRankingsResponse{
		OrganizationID: pgutil.UUIDToString(org.ID),
		Rankings:       entries,
		Total:          total,
		Limit:          int(params.Limit),
		Offset:         int(params.Offset),
	}, nil
}

// ListTeamRankings returns paginated all-time team rankings for an org.
func (s *Service) ListTeamRankings(ctx context.Context, orgSlug string, params ListParams) (*TeamRankingsResponse, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	if params.Limit <= 0 || params.Limit > MaxListLimit {
		params.Limit = DefaultListLimit
	}
	if params.Offset < 0 {
		params.Offset = 0
	}

	rows, err := s.repo.ListTeamRankings(ctx, org.ID, params)
	if err != nil {
		return nil, err
	}

	total, err := s.repo.CountTeamRankings(ctx, org.ID)
	if err != nil {
		return nil, err
	}

	entries := make([]TeamRankingEntry, len(rows))
	for i, r := range rows {
		entries[i] = TeamRankingEntry{
			Rank:              int(r.Rank),
			TeamID:            pgutil.UUIDToString(r.TeamID),
			TeamName:          r.TeamName,
			TournamentsPlayed: int(r.TournamentsPlayed),
			TournamentsWon:    int(r.TournamentsWon),
			PodiumFinishes:    int(r.PodiumFinishes),
			TotalMatches:      int(r.TotalMatches),
			TotalWins:         int(r.TotalWins),
			TotalDraws:        int(r.TotalDraws),
			TotalLosses:       int(r.TotalLosses),
			TotalPoints:       int(r.TotalPoints),
			WinRate:           winRate(int(r.TotalWins), int(r.TotalMatches)),
			ScoreFor:          int(r.ScoreFor),
			ScoreAgainst:      int(r.ScoreAgainst),
		}
	}

	return &TeamRankingsResponse{
		OrganizationID: pgutil.UUIDToString(org.ID),
		Rankings:       entries,
		Total:          total,
		Limit:          int(params.Limit),
		Offset:         int(params.Offset),
	}, nil
}

// winRate returns wins/matches as a float64 in [0, 1].
// Returns 0 when totalMatches is 0 to avoid division by zero.
func winRate(wins, totalMatches int) float64 {
	if totalMatches == 0 {
		return 0
	}
	return float64(wins) / float64(totalMatches)
}

