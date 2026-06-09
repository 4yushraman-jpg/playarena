package rankings

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
)

// Repository provides data access for the rankings domain.
type Repository struct {
	queries *db.Queries
	pool    *pgxpool.Pool
}

// NewRepository constructs a Repository.
func NewRepository(queries *db.Queries, pool *pgxpool.Pool) *Repository {
	return &Repository{queries: queries, pool: pool}
}

// ── reads ─────────────────────────────────────────────────────────────────────

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

func (r *Repository) ListPlayerRankings(ctx context.Context, orgID pgtype.UUID, params ListParams) ([]db.ListPlayerRankingsRow, error) {
	return r.queries.ListPlayerRankings(ctx, db.ListPlayerRankingsParams{
		OrganizationID: orgID,
		Limit:          params.Limit,
		Offset:         params.Offset,
	})
}

func (r *Repository) CountPlayerRankings(ctx context.Context, orgID pgtype.UUID) (int64, error) {
	return r.queries.CountPlayerRankings(ctx, orgID)
}

func (r *Repository) ListTeamRankings(ctx context.Context, orgID pgtype.UUID, params ListParams) ([]db.ListTeamRankingsRow, error) {
	return r.queries.ListTeamRankings(ctx, db.ListTeamRankingsParams{
		OrganizationID: orgID,
		Limit:          params.Limit,
		Offset:         params.Offset,
	})
}

func (r *Repository) CountTeamRankings(ctx context.Context, orgID pgtype.UUID) (int64, error) {
	return r.queries.CountTeamRankings(ctx, orgID)
}

// ── snapshot writes ───────────────────────────────────────────────────────────

// SnapshotPlayerStats upserts final standings for a completed individual tournament.
// Idempotent: retrying after a crash produces the same result.
func (r *Repository) SnapshotPlayerStats(ctx context.Context, orgID, tournamentID pgtype.UUID, rows []StatsRow) error {
	for _, row := range rows {
		playerID, err := pgutil.ParseUUID(row.ParticipantID)
		if err != nil {
			continue // skip malformed ID rather than aborting the whole snapshot
		}
		if err := r.queries.UpsertPlayerTournamentStats(ctx, db.UpsertPlayerTournamentStatsParams{
			PlayerID:       playerID,
			TournamentID:   tournamentID,
			OrganizationID: orgID,
			Position:       int32(row.Position),
			MatchesPlayed:  int32(row.Played),
			MatchesWon:     int32(row.Wins),
			MatchesDrawn:   int32(row.Draws),
			MatchesLost:    int32(row.Losses),
			Points:         int32(row.Points),
			ScoreFor:       int32(row.ScoreFor),
			ScoreAgainst:   int32(row.ScoreAgainst),
		}); err != nil {
			return err
		}
	}
	return nil
}

// SnapshotTeamStats upserts final standings for a completed team tournament.
// Idempotent: retrying after a crash produces the same result.
func (r *Repository) SnapshotTeamStats(ctx context.Context, orgID, tournamentID pgtype.UUID, rows []StatsRow) error {
	for _, row := range rows {
		teamID, err := pgutil.ParseUUID(row.ParticipantID)
		if err != nil {
			continue
		}
		if err := r.queries.UpsertTeamTournamentStats(ctx, db.UpsertTeamTournamentStatsParams{
			TeamID:         teamID,
			TournamentID:   tournamentID,
			OrganizationID: orgID,
			Position:       int32(row.Position),
			MatchesPlayed:  int32(row.Played),
			MatchesWon:     int32(row.Wins),
			MatchesDrawn:   int32(row.Draws),
			MatchesLost:    int32(row.Losses),
			Points:         int32(row.Points),
			ScoreFor:       int32(row.ScoreFor),
			ScoreAgainst:   int32(row.ScoreAgainst),
		}); err != nil {
			return err
		}
	}
	return nil
}
