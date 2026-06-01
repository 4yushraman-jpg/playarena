package matches

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
)

// allowedTransitions defines the permitted match status moves for Phase 8A.
// Terminal states (completed, cancelled, abandoned) have empty target sets.
var allowedTransitions = map[db.MatchStatus][]db.MatchStatus{
	db.MatchStatusScheduled: {
		db.MatchStatusLive,
		db.MatchStatusCancelled,
	},
	db.MatchStatusLive: {
		db.MatchStatusCompleted,
		db.MatchStatusAbandoned,
		db.MatchStatusCancelled,
	},
	db.MatchStatusCompleted: {},
	db.MatchStatusCancelled: {},
	db.MatchStatusAbandoned: {},
}

// terminalStatuses is the set of states from which no further transitions
// are permitted (used by the Update guard check).
var terminalStatuses = map[db.MatchStatus]bool{
	db.MatchStatusCompleted: true,
	db.MatchStatusCancelled: true,
	db.MatchStatusAbandoned: true,
}

// tournamentLockStatuses is the set of target statuses that require a FOR SHARE
// lock on the parent tournament row to prevent concurrent cancellation races.
var tournamentLockStatuses = map[db.MatchStatus]bool{
	db.MatchStatusLive:      true,
	db.MatchStatusCompleted: true,
	db.MatchStatusAbandoned: true,
}

// Service implements match use-cases.
type Service struct {
	repo *Repository
	log  *slog.Logger
}

// NewService constructs a Service.
func NewService(repo *Repository, log *slog.Logger) *Service {
	return &Service{repo: repo, log: log}
}

// ── public methods ────────────────────────────────────────────────────────────

// Create schedules a new match fixture within a tournament.
//
// Business rules enforced (in order):
//  1. BOLA guard: actorOrgID must match the URL org or be empty (platform admin).
//  2. Tournament must belong to the URL org.
//  3. Tournament must be in ongoing status (pre-tx; also re-checked inside tx).
//  4. scheduled_at must be a valid non-zero RFC3339 timestamp.
//  5. Participant type must match the tournament's participant_type.
//  6. Both home and away participants must be provided (no TBD slots via API).
//  7. Home and away participants must be different.
//  8. Each participant must belong to the URL org (cross-org guard).
//  9. Each participant must hold an approved registration for the tournament.
func (s *Service) Create(
	ctx context.Context,
	orgSlug string,
	req CreateRequest,
	actorID, actorOrgID string,
) (*Response, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	if err := assertOrgOwnership(actorOrgID, pgutil.UUIDToString(org.ID)); err != nil {
		return nil, err
	}

	tid, err := pgutil.ParseUUID(req.TournamentID)
	if err != nil {
		return nil, ErrInvalidTournamentID
	}

	tournament, err := s.repo.GetTournamentByID(ctx, tid, org.ID)
	if err != nil {
		return nil, err
	}

	// Pre-transaction status check; CreateWithAudit re-validates under a
	// FOR SHARE lock to close the tournament-cancellation race window.
	if tournament.Status != db.TournamentStatusOngoing {
		return nil, ErrTournamentNotOngoing
	}

	scheduledAt, err := parseRequiredTimestamp(req.ScheduledAt)
	if err != nil {
		return nil, err
	}

	homeTeamUID := pgutil.ParseOptionalUUID(derefStr(req.HomeTeamID))
	awayTeamUID := pgutil.ParseOptionalUUID(derefStr(req.AwayTeamID))
	homePlayerUID := pgutil.ParseOptionalUUID(derefStr(req.HomePlayerID))
	awayPlayerUID := pgutil.ParseOptionalUUID(derefStr(req.AwayPlayerID))

	if err := validateParticipantType(tournament.ParticipantType,
		homeTeamUID, awayTeamUID, homePlayerUID, awayPlayerUID); err != nil {
		return nil, err
	}

	if err := validateNoDuplicates(homeTeamUID, awayTeamUID, homePlayerUID, awayPlayerUID); err != nil {
		return nil, err
	}

	if err := s.validateParticipantEligibility(ctx, tid, org.ID,
		homeTeamUID, awayTeamUID, homePlayerUID, awayPlayerUID); err != nil {
		return nil, err
	}

	actorUID, err := pgutil.ParseUUID(actorID)
	if err != nil {
		return nil, errors.New("invalid actor user id")
	}

	m, err := s.repo.CreateWithAudit(ctx, createMatchTxParams{
		createParams: db.CreateMatchParams{
			TournamentID:   tid,
			OrganizationID: org.ID,
			RoundNumber:    req.RoundNumber,
			RoundName:      req.RoundName,
			MatchNumber:    req.MatchNumber,
			HomeTeamID:     homeTeamUID,
			AwayTeamID:     awayTeamUID,
			HomePlayerID:   homePlayerUID,
			AwayPlayerID:   awayPlayerUID,
			Venue:          req.Venue,
			ScheduledAt:    scheduledAt,
			Status:         db.MatchStatusScheduled,
			Notes:          req.Notes,
		},
		actorID:        actorUID,
		tournamentID:   tid,
		organizationID: org.ID,
	})
	if err != nil {
		return nil, err
	}
	return matchToResponse(m), nil
}

// List returns a paginated list of matches for an organization.
// No ownership check: any authenticated user may list matches.
func (s *Service) List(ctx context.Context, orgSlug string, params ListParams) (*ListResponse, error) {
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

	// Validate optional tournament_id filter is a valid UUID if provided.
	if params.TournamentFilter != nil && *params.TournamentFilter != "" {
		if _, err := pgutil.ParseUUID(*params.TournamentFilter); err != nil {
			return nil, ErrInvalidTournamentID
		}
	}

	ms, err := s.repo.List(ctx, org.ID, params)
	if err != nil {
		return nil, err
	}

	total, err := s.repo.Count(ctx, org.ID, params)
	if err != nil {
		return nil, err
	}

	resp := make([]Response, len(ms))
	for i := range ms {
		resp[i] = *matchToResponse(&ms[i])
	}
	return &ListResponse{
		Matches: resp,
		Total:   total,
		Limit:   int(params.Limit),
		Offset:  int(params.Offset),
	}, nil
}

// GetByID retrieves a single match by UUID within an organization.
// No ownership check: any authenticated user may read match details.
// Cancelled and completed matches are intentionally returned.
func (s *Service) GetByID(ctx context.Context, orgSlug, matchID string) (*Response, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	mid, err := pgutil.ParseUUID(matchID)
	if err != nil {
		return nil, ErrMatchNotFound
	}

	m, err := s.repo.GetByID(ctx, mid, org.ID)
	if err != nil {
		return nil, err
	}
	return matchToResponse(m), nil
}

// Update applies a partial update to a match.
// Status changes are validated against the allowed transition table.
// BOLA guard: actorOrgID must match the URL org or be empty (platform admin).
func (s *Service) Update(
	ctx context.Context,
	orgSlug, matchID string,
	req UpdateRequest,
	actorID, actorOrgID string,
) (*Response, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	if err := assertOrgOwnership(actorOrgID, pgutil.UUIDToString(org.ID)); err != nil {
		return nil, err
	}

	mid, err := pgutil.ParseUUID(matchID)
	if err != nil {
		return nil, ErrMatchNotFound
	}

	current, err := s.repo.GetByID(ctx, mid, org.ID)
	if err != nil {
		return nil, err
	}

	if terminalStatuses[current.Status] {
		return nil, ErrMatchNotUpdatable
	}

	actorUID, err := pgutil.ParseUUID(actorID)
	if err != nil {
		return nil, errors.New("invalid actor user id")
	}

	oldData, err := json.Marshal(matchToResponse(current))
	if err != nil {
		return nil, err
	}

	// Start with current state; apply non-nil request fields over it.
	params := db.UpdateMatchParams{
		ID:             current.ID,
		OrganizationID: current.OrganizationID,
		RoundNumber:    current.RoundNumber,
		RoundName:      current.RoundName,
		MatchNumber:    current.MatchNumber,
		HomeTeamID:     current.HomeTeamID,
		AwayTeamID:     current.AwayTeamID,
		HomePlayerID:   current.HomePlayerID,
		AwayPlayerID:   current.AwayPlayerID,
		Venue:          current.Venue,
		ScheduledAt:    current.ScheduledAt,
		StartedAt:      current.StartedAt,
		EndedAt:        current.EndedAt,
		Status:         current.Status,
		WinnerTeamID:   current.WinnerTeamID,
		WinnerPlayerID: current.WinnerPlayerID,
		Notes:          current.Notes,
	}

	if req.RoundNumber != nil {
		params.RoundNumber = req.RoundNumber
	}
	if req.RoundName != nil {
		params.RoundName = req.RoundName
	}
	if req.MatchNumber != nil {
		params.MatchNumber = req.MatchNumber
	}
	if req.Venue != nil {
		params.Venue = req.Venue
	}
	if req.Notes != nil {
		params.Notes = req.Notes
	}
	if req.ScheduledAt != nil {
		ts, err := parseOptionalTimestamp(req.ScheduledAt)
		if err != nil {
			return nil, err
		}
		params.ScheduledAt = ts
	}

	// Track whether participant fields are being changed.
	participantsChanged := false
	if req.HomeTeamID != nil {
		params.HomeTeamID = pgutil.ParseOptionalUUID(*req.HomeTeamID)
		participantsChanged = true
	}
	if req.AwayTeamID != nil {
		params.AwayTeamID = pgutil.ParseOptionalUUID(*req.AwayTeamID)
		participantsChanged = true
	}
	if req.HomePlayerID != nil {
		params.HomePlayerID = pgutil.ParseOptionalUUID(*req.HomePlayerID)
		participantsChanged = true
	}
	if req.AwayPlayerID != nil {
		params.AwayPlayerID = pgutil.ParseOptionalUUID(*req.AwayPlayerID)
		participantsChanged = true
	}

	// Re-validate participant rules when any participant field changed.
	if participantsChanged {
		tournament, err := s.repo.GetTournamentByID(ctx, current.TournamentID, org.ID)
		if err != nil {
			return nil, err
		}
		if err := validateParticipantType(tournament.ParticipantType,
			params.HomeTeamID, params.AwayTeamID,
			params.HomePlayerID, params.AwayPlayerID); err != nil {
			return nil, err
		}
		if err := validateNoDuplicates(params.HomeTeamID, params.AwayTeamID,
			params.HomePlayerID, params.AwayPlayerID); err != nil {
			return nil, err
		}
		if err := s.validateParticipantEligibility(ctx, current.TournamentID, org.ID,
			params.HomeTeamID, params.AwayTeamID,
			params.HomePlayerID, params.AwayPlayerID); err != nil {
			return nil, err
		}
	}

	// Status transition validation and timestamp side-effects.
	lockTournament := false
	if req.Status != nil {
		newStatus, err := parseMatchStatus(*req.Status)
		if err != nil {
			return nil, err
		}
		if err := validateStatusTransition(current.Status, newStatus); err != nil {
			return nil, err
		}
		params.Status = newStatus

		// Stamp timestamps on lifecycle transitions.
		if newStatus == db.MatchStatusLive && !params.StartedAt.Valid {
			params.StartedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
		}
		if (newStatus == db.MatchStatusCompleted || newStatus == db.MatchStatusAbandoned) &&
			!params.EndedAt.Valid {
			params.EndedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
		}

		// Tournament status must be ongoing when advancing a match's lifecycle.
		if tournamentLockStatuses[newStatus] {
			lockTournament = true
		}
	}

	// Winner validation: winner may only be set when resulting status is completed.
	winnerBeingSet := req.WinnerTeamID != nil || req.WinnerPlayerID != nil
	if winnerBeingSet {
		if params.Status != db.MatchStatusCompleted {
			return nil, ErrWinnerNotAllowed
		}
	}
	if req.WinnerTeamID != nil {
		params.WinnerTeamID = pgutil.ParseOptionalUUID(*req.WinnerTeamID)
	}
	if req.WinnerPlayerID != nil {
		params.WinnerPlayerID = pgutil.ParseOptionalUUID(*req.WinnerPlayerID)
	}

	// Validate winner is one of the match participants (when set).
	if params.WinnerTeamID.Valid {
		if !uuidEquals(params.WinnerTeamID, params.HomeTeamID) &&
			!uuidEquals(params.WinnerTeamID, params.AwayTeamID) {
			return nil, ErrWinnerNotParticipant
		}
	}
	if params.WinnerPlayerID.Valid {
		if !uuidEquals(params.WinnerPlayerID, params.HomePlayerID) &&
			!uuidEquals(params.WinnerPlayerID, params.AwayPlayerID) {
			return nil, ErrWinnerNotParticipant
		}
	}

	updated, err := s.repo.UpdateWithAudit(ctx, updateMatchTxParams{
		updateParams:   params,
		actorID:        actorUID,
		oldData:        oldData,
		lockTournament: lockTournament,
		tournamentID:   current.TournamentID,
		organizationID: current.OrganizationID,
	})
	if err != nil {
		return nil, err
	}
	return matchToResponse(updated), nil
}

// Delete soft-cancels a match (status → cancelled).
// Only non-terminal matches can be cancelled.
// BOLA guard: actorOrgID must match the URL org or be empty (platform admin).
func (s *Service) Delete(
	ctx context.Context,
	orgSlug, matchID string,
	actorID, actorOrgID string,
) error {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return err
	}

	if err := assertOrgOwnership(actorOrgID, pgutil.UUIDToString(org.ID)); err != nil {
		return err
	}

	mid, err := pgutil.ParseUUID(matchID)
	if err != nil {
		return ErrMatchNotFound
	}

	current, err := s.repo.GetByID(ctx, mid, org.ID)
	if err != nil {
		return err
	}

	if current.Status == db.MatchStatusCancelled {
		return ErrMatchAlreadyCancelled
	}
	if terminalStatuses[current.Status] {
		// completed and abandoned are terminal; cancellation is not permitted.
		return ErrMatchNotUpdatable
	}

	actorUID, err := pgutil.ParseUUID(actorID)
	if err != nil {
		return errors.New("invalid actor user id")
	}

	oldData, err := json.Marshal(matchToResponse(current))
	if err != nil {
		return err
	}

	return s.repo.CancelWithAudit(ctx, cancelMatchTxParams{
		id:      current.ID,
		orgID:   current.OrganizationID,
		actorID: actorUID,
		oldData: oldData,
	})
}

// ── helpers ───────────────────────────────────────────────────────────────────

func assertOrgOwnership(actorOrgID, targetOrgID string) error {
	if actorOrgID == "" {
		return nil // platform admin
	}
	if actorOrgID != targetOrgID {
		return ErrForbidden
	}
	return nil
}

// validateParticipantType enforces that the provided UUIDs are consistent with
// the tournament's participant_type:
//   - team tournaments:       homeTeam + awayTeam must be set; player IDs must be absent.
//   - individual tournaments: homePlayer + awayPlayer must be set; team IDs must be absent.
func validateParticipantType(
	pt db.ParticipantType,
	homeTeam, awayTeam, homePlayer, awayPlayer pgtype.UUID,
) error {
	switch pt {
	case db.ParticipantTypeTeam:
		if homePlayer.Valid || awayPlayer.Valid {
			return ErrMixedParticipantTypes
		}
		if !homeTeam.Valid || !awayTeam.Valid {
			return ErrMissingParticipants
		}
	case db.ParticipantTypeIndividual:
		if homeTeam.Valid || awayTeam.Valid {
			return ErrMixedParticipantTypes
		}
		if !homePlayer.Valid || !awayPlayer.Valid {
			return ErrMissingParticipants
		}
	}
	return nil
}

// validateNoDuplicates rejects matches where home and away are the same entity.
func validateNoDuplicates(homeTeam, awayTeam, homePlayer, awayPlayer pgtype.UUID) error {
	if homeTeam.Valid && awayTeam.Valid && uuidEquals(homeTeam, awayTeam) {
		return ErrDuplicateParticipants
	}
	if homePlayer.Valid && awayPlayer.Valid && uuidEquals(homePlayer, awayPlayer) {
		return ErrDuplicateParticipants
	}
	return nil
}

// validateParticipantEligibility checks cross-org membership and approved
// registration for each non-null participant UUID.
func (s *Service) validateParticipantEligibility(
	ctx context.Context,
	tournamentID, orgID pgtype.UUID,
	homeTeam, awayTeam, homePlayer, awayPlayer pgtype.UUID,
) error {
	for _, teamUID := range []pgtype.UUID{homeTeam, awayTeam} {
		if !teamUID.Valid {
			continue
		}
		if _, err := s.repo.GetTeamByID(ctx, teamUID, orgID); err != nil {
			return ErrParticipantCrossOrg
		}
		ok, err := s.repo.HasApprovedRegistrationByTeam(ctx, tournamentID, teamUID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrParticipantNotRegistered
		}
	}
	for _, playerUID := range []pgtype.UUID{homePlayer, awayPlayer} {
		if !playerUID.Valid {
			continue
		}
		if _, err := s.repo.GetPlayerByID(ctx, playerUID, orgID); err != nil {
			return ErrParticipantCrossOrg
		}
		ok, err := s.repo.HasApprovedRegistrationByPlayer(ctx, tournamentID, playerUID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrParticipantNotRegistered
		}
	}
	return nil
}

func parseMatchStatus(s string) (db.MatchStatus, error) {
	st := db.MatchStatus(strings.ToLower(strings.TrimSpace(s)))
	switch st {
	case db.MatchStatusScheduled, db.MatchStatusLive, db.MatchStatusCompleted,
		db.MatchStatusCancelled, db.MatchStatusAbandoned:
		return st, nil
	}
	return "", ErrInvalidStatus
}

func validateStatusTransition(from, to db.MatchStatus) error {
	targets, ok := allowedTransitions[from]
	if !ok {
		return ErrInvalidStatusTransition
	}
	for _, t := range targets {
		if t == to {
			return nil
		}
	}
	return fmt.Errorf("%w: %s → %s", ErrInvalidStatusTransition, from, to)
}

// parseRequiredTimestamp parses a mandatory RFC3339 timestamp string.
// Returns ErrInvalidTimestamp for empty, unparseable, or zero-value inputs.
func parseRequiredTimestamp(s string) (pgtype.Timestamptz, error) {
	if s == "" {
		return pgtype.Timestamptz{}, ErrInvalidTimestamp
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return pgtype.Timestamptz{}, ErrInvalidTimestamp
	}
	if t.IsZero() {
		return pgtype.Timestamptz{}, ErrInvalidTimestamp
	}
	return pgtype.Timestamptz{Time: t.UTC(), Valid: true}, nil
}

// parseOptionalTimestamp parses an optional RFC3339 timestamp pointer.
// A nil pointer or empty string returns a zero Timestamptz (SQL NULL).
func parseOptionalTimestamp(s *string) (pgtype.Timestamptz, error) {
	if s == nil || *s == "" {
		return pgtype.Timestamptz{}, nil
	}
	t, err := time.Parse(time.RFC3339, *s)
	if err != nil {
		return pgtype.Timestamptz{}, ErrInvalidTimestamp
	}
	return pgtype.Timestamptz{Time: t.UTC(), Valid: true}, nil
}

// uuidEquals reports whether two pgtype.UUIDs represent the same non-null value.
func uuidEquals(a, b pgtype.UUID) bool {
	return a.Valid && b.Valid && a.Bytes == b.Bytes
}

// matchToResponse converts a db.Match row to the API response struct.
func matchToResponse(m *db.Match) *Response {
	return &Response{
		ID:             pgutil.UUIDToString(m.ID),
		TournamentID:   pgutil.UUIDToString(m.TournamentID),
		OrganizationID: pgutil.UUIDToString(m.OrganizationID),
		RoundNumber:    m.RoundNumber,
		RoundName:      m.RoundName,
		MatchNumber:    m.MatchNumber,
		HomeTeamID:     uuidStringPtr(m.HomeTeamID),
		AwayTeamID:     uuidStringPtr(m.AwayTeamID),
		HomePlayerID:   uuidStringPtr(m.HomePlayerID),
		AwayPlayerID:   uuidStringPtr(m.AwayPlayerID),
		Venue:          m.Venue,
		ScheduledAt:    tsStringPtr(m.ScheduledAt),
		StartedAt:      tsStringPtr(m.StartedAt),
		EndedAt:        tsStringPtr(m.EndedAt),
		Status:         string(m.Status),
		WinnerTeamID:   uuidStringPtr(m.WinnerTeamID),
		WinnerPlayerID: uuidStringPtr(m.WinnerPlayerID),
		IsWalkover:     m.IsWalkover,
		Notes:          m.Notes,
		CreatedAt:      m.CreatedAt.Time.UTC().Format(time.RFC3339),
		UpdatedAt:      m.UpdatedAt.Time.UTC().Format(time.RFC3339),
	}
}

func uuidStringPtr(uid pgtype.UUID) *string {
	if !uid.Valid {
		return nil
	}
	s := pgutil.UUIDToString(uid)
	return &s
}

func tsStringPtr(ts pgtype.Timestamptz) *string {
	if !ts.Valid {
		return nil
	}
	s := ts.Time.UTC().Format(time.RFC3339)
	return &s
}
