package public

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
)

// Repository is the read-only, principal-less data access for the public path.
// It exposes ONLY whitelisted public queries plus reuse of pure batch-read
// queries (name lookups, completed-match standings feed). It never calls an
// org-scoped service and never receives a principal — authorization is the SQL
// visibility gate in GetPublicTournament.
type Repository struct {
	queries *db.Queries
}

func NewRepository(queries *db.Queries) *Repository {
	return &Repository{queries: queries}
}

// GetTournament resolves a publicly-eligible tournament by (org slug, tournament
// slug). The visibility gate lives in the query; a row that fails it (private,
// draft, inactive org, or non-existent) yields ErrNotFound — never a distinction.
func (r *Repository) GetTournament(ctx context.Context, orgSlug, tournamentSlug string) (db.GetPublicTournamentRow, error) {
	row, err := r.queries.GetPublicTournament(ctx, db.GetPublicTournamentParams{
		Slug:   orgSlug,
		Slug_2: tournamentSlug,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return db.GetPublicTournamentRow{}, ErrNotFound
		}
		return db.GetPublicTournamentRow{}, err
	}
	return row, nil
}

func (r *Repository) ListMatches(ctx context.Context, tournamentID, orgID pgtype.UUID) ([]db.ListPublicMatchesRow, error) {
	return r.queries.ListPublicMatches(ctx, db.ListPublicMatchesParams{
		TournamentID:   tournamentID,
		OrganizationID: orgID,
	})
}

func (r *Repository) GetMatch(ctx context.Context, matchID, tournamentID, orgID pgtype.UUID) (db.GetPublicMatchRow, error) {
	row, err := r.queries.GetPublicMatch(ctx, db.GetPublicMatchParams{
		ID:             matchID,
		TournamentID:   tournamentID,
		OrganizationID: orgID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return db.GetPublicMatchRow{}, ErrNotFound
		}
		return db.GetPublicMatchRow{}, err
	}
	return row, nil
}

func (r *Repository) ListMatchEvents(ctx context.Context, matchID, orgID pgtype.UUID) ([]db.ListPublicMatchEventsRow, error) {
	return r.queries.ListPublicMatchEvents(ctx, db.ListPublicMatchEventsParams{
		MatchID:        matchID,
		OrganizationID: orgID,
	})
}

// CompletedMatches feeds the standings engine (reuses the same query the
// organizer standings use: status IN ('completed','walkover')).
func (r *Repository) CompletedMatches(ctx context.Context, tournamentID, orgID pgtype.UUID) ([]db.ListCompletedMatchesByTournamentRow, error) {
	return r.queries.ListCompletedMatchesByTournament(ctx, db.ListCompletedMatchesByTournamentParams{
		TournamentID:   tournamentID,
		OrganizationID: orgID,
	})
}

func (r *Repository) StandingsRegistrations(ctx context.Context, tournamentID pgtype.UUID) ([]db.ListStandingsRegistrationsRow, error) {
	return r.queries.ListStandingsRegistrations(ctx, tournamentID)
}

// TournamentSettings reads the settings JSONB for an already-gated tournament,
// used ONLY server-side to derive the public point system. The raw settings are
// never serialized into a public response.
func (r *Repository) TournamentSettings(ctx context.Context, tournamentID, orgID pgtype.UUID) ([]byte, error) {
	t, err := r.queries.GetTournamentByID(ctx, db.GetTournamentByIDParams{
		ID:             tournamentID,
		OrganizationID: orgID,
	})
	if err != nil {
		return nil, err
	}
	return t.Settings, nil
}

func (r *Repository) TeamNames(ctx context.Context, ids []pgtype.UUID) (map[string]string, error) {
	rows, err := r.queries.ListTeamNamesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, row := range rows {
		out[uuidString(row.ID)] = row.Name
	}
	return out, nil
}

func (r *Repository) PlayerNames(ctx context.Context, ids []pgtype.UUID) (map[string]string, error) {
	rows, err := r.queries.ListPlayerNamesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, row := range rows {
		out[uuidString(row.ID)] = row.DisplayName
	}
	return out, nil
}
