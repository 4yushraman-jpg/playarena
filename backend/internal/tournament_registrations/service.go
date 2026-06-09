package tournament_registrations

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
	"github.com/4yushraman-jpg/playarena/internal/notifications"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
)

// allowedTransitions defines the permitted registration status moves.
// Terminal states (rejected, withdrawn, disqualified) have empty target sets.
var allowedTransitions = map[db.RegistrationStatus][]db.RegistrationStatus{
	db.RegistrationStatusPending: {
		db.RegistrationStatusApproved,
		db.RegistrationStatusRejected,
		db.RegistrationStatusWithdrawn,
	},
	db.RegistrationStatusApproved: {
		db.RegistrationStatusWithdrawn,
		db.RegistrationStatusDisqualified,
	},
	db.RegistrationStatusRejected:     {},
	db.RegistrationStatusWithdrawn:    {},
	db.RegistrationStatusDisqualified: {},
}

// Service implements tournament registration use-cases.
type Service struct {
	repo     *Repository
	log      *slog.Logger
	notifSvc *notifications.Service
}

// NewService constructs a Service.
func NewService(repo *Repository, log *slog.Logger, notifSvc *notifications.Service) *Service {
	return &Service{repo: repo, log: log, notifSvc: notifSvc}
}

// ── public methods ────────────────────────────────────────────────────────────

// Register submits a new registration for a team in a tournament.
//
// Enforces all seven business rules in order:
//  1. Tournament and team belong to the URL org (multi-tenant safety).
//  2. Tournament is in registration_open status.
//  3. Current time is within the registration window.
//  4. Team is not already registered.
//  5. Team is active and belongs to the org.
//  6. Team has at least one active member.
//  7. Tournament has not reached max_participants capacity.
//
// BOLA guard: actorOrgID must match the URL org or be empty (platform admin).
func (s *Service) Register(
	ctx context.Context,
	orgSlug, tournamentID string,
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

	tid, err := pgutil.ParseUUID(tournamentID)
	if err != nil {
		return nil, ErrTournamentNotFound
	}

	// Tournament must belong to the URL org.
	tournament, err := s.repo.GetTournamentByID(ctx, tid, org.ID)
	if err != nil {
		return nil, err
	}

	// Rule 2: Tournament must be in registration_open status.
	if tournament.Status != db.TournamentStatusRegistrationOpen {
		return nil, ErrRegistrationClosed
	}

	// Rule 3: Current time must be within the registration window.
	if err := validateRegistrationWindow(tournament); err != nil {
		return nil, err
	}

	teamUID, err := pgutil.ParseUUID(req.TeamID)
	if err != nil {
		return nil, ErrTeamNotFound
	}

	// Rules 1 & 5: Team must exist and belong to the URL org.
	team, err := s.repo.GetTeamByID(ctx, teamUID, org.ID)
	if err != nil {
		if errors.Is(err, ErrTeamNotFound) {
			return nil, ErrCrossOrgRegistration
		}
		return nil, err
	}

	// Rule 5: Team must be active.
	if team.Status != db.TeamStatusActive {
		return nil, ErrTeamNotActive
	}

	// Rule 4: No duplicate registration for this (tournament, team) pair.
	existing, err := s.repo.GetByTeam(ctx, tid, teamUID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrAlreadyRegistered
	}

	// Rule 6: Team must have at least one active member.
	hasMembers, err := s.repo.HasActiveMembers(ctx, teamUID, org.ID)
	if err != nil {
		return nil, err
	}
	if !hasMembers {
		return nil, ErrEmptyTeam
	}

	// Rule 7 — Capacity enforcement is deferred to CreateWithAudit, where it
	// runs under a tournament row lock (SELECT … FOR UPDATE). Performing the
	// count and insert inside the same locked transaction prevents concurrent
	// requests from both reading "capacity available" and both inserting.
	actorUID, err := pgutil.ParseUUID(actorID)
	if err != nil {
		return nil, errors.New("invalid actor user id")
	}

	reg, err := s.repo.CreateWithAudit(ctx, createRegistrationTxParams{
		createParams: db.CreateRegistrationParams{
			TournamentID:   tid,
			OrganizationID: org.ID,
			TeamID:         teamUID,
			PlayerID:       pgtype.UUID{}, // null for team registrations
			RegisteredBy:   actorUID,
			Notes:          req.Notes,
		},
		actorID:         actorUID,
		maxParticipants: tournament.MaxParticipants,
	})
	if err != nil {
		return nil, err
	}
	return registrationToResponse(reg), nil
}

// List returns a paginated list of registrations for a tournament.
// No ownership check: any authenticated user may list registrations.
func (s *Service) List(ctx context.Context, orgSlug, tournamentID string, params ListParams) (*ListResponse, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	tid, err := pgutil.ParseUUID(tournamentID)
	if err != nil {
		return nil, ErrTournamentNotFound
	}
	if _, err := s.repo.GetTournamentByID(ctx, tid, org.ID); err != nil {
		return nil, err
	}

	if params.Limit <= 0 || params.Limit > MaxListLimit {
		params.Limit = DefaultListLimit
	}
	if params.Offset < 0 {
		params.Offset = 0
	}

	regs, err := s.repo.List(ctx, tid, params)
	if err != nil {
		return nil, err
	}

	total, err := s.repo.Count(ctx, tid, params)
	if err != nil {
		return nil, err
	}

	resp := make([]Response, len(regs))
	for i := range regs {
		resp[i] = *registrationToResponse(&regs[i])
	}
	return &ListResponse{
		Registrations: resp,
		Total:         total,
		Limit:         int(params.Limit),
		Offset:        int(params.Offset),
	}, nil
}

// GetByID retrieves a single registration.
// No ownership check: any authenticated user may read registration details.
func (s *Service) GetByID(ctx context.Context, orgSlug, tournamentID, registrationID string) (*Response, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	tid, err := pgutil.ParseUUID(tournamentID)
	if err != nil {
		return nil, ErrTournamentNotFound
	}
	if _, err := s.repo.GetTournamentByID(ctx, tid, org.ID); err != nil {
		return nil, err
	}

	rid, err := pgutil.ParseUUID(registrationID)
	if err != nil {
		return nil, ErrRegistrationNotFound
	}

	reg, err := s.repo.GetByID(ctx, rid, tid)
	if err != nil {
		return nil, err
	}
	return registrationToResponse(reg), nil
}

// Update applies a partial update to a registration (status, notes, seed_number).
// Status changes are validated against the allowed transition table.
// When transitioning to approved, approved_by and approved_at are stamped.
// BOLA guard: actorOrgID must match the URL org or be empty (platform admin).
func (s *Service) Update(
	ctx context.Context,
	orgSlug, tournamentID, registrationID string,
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

	tid, err := pgutil.ParseUUID(tournamentID)
	if err != nil {
		return nil, ErrTournamentNotFound
	}
	if _, err := s.repo.GetTournamentByID(ctx, tid, org.ID); err != nil {
		return nil, err
	}

	rid, err := pgutil.ParseUUID(registrationID)
	if err != nil {
		return nil, ErrRegistrationNotFound
	}

	current, err := s.repo.GetByID(ctx, rid, tid)
	if err != nil {
		return nil, err
	}

	actorUID, err := pgutil.ParseUUID(actorID)
	if err != nil {
		return nil, errors.New("invalid actor user id")
	}

	oldData, err := json.Marshal(registrationToResponse(current))
	if err != nil {
		return nil, err
	}

	params := db.UpdateRegistrationParams{
		ID:           current.ID,
		TournamentID: current.TournamentID,
		Status:       current.Status,
		SeedNumber:   current.SeedNumber,
		Notes:        current.Notes,
		ApprovedBy: current.ApprovedBy,
		Column7:    current.ApprovedAt,
	}

	if req.SeedNumber != nil {
		params.SeedNumber = req.SeedNumber
	}
	if req.Notes != nil {
		params.Notes = req.Notes
	}
	if req.Status != nil {
		newStatus, err := parseRegistrationStatus(*req.Status)
		if err != nil {
			return nil, err
		}
		if err := validateStatusTransition(current.Status, newStatus); err != nil {
			return nil, err
		}
		params.Status = newStatus

		// Stamp approval fields when transitioning to approved.
		if newStatus == db.RegistrationStatusApproved {
			params.ApprovedBy = actorUID
			params.Column7 = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
		}
	}

	updated, err := s.repo.UpdateWithAudit(ctx, updateRegistrationTxParams{
		updateParams:   params,
		actorID:        actorUID,
		orgID:          org.ID,
		oldData:        oldData,
		previousStatus: current.Status,
	})
	if err != nil {
		return nil, err
	}

	// Synchronous post-commit drain.
	s.notifSvc.DrainOutbox(ctx, org.ID, s.log)

	return registrationToResponse(updated), nil
}

// Withdraw soft-deletes a registration by setting its status to withdrawn.
// Only pending and approved registrations can be withdrawn.
// BOLA guard: actorOrgID must match the URL org or be empty (platform admin).
func (s *Service) Withdraw(
	ctx context.Context,
	orgSlug, tournamentID, registrationID string,
	actorID, actorOrgID string,
) error {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return err
	}

	if err := assertOrgOwnership(actorOrgID, pgutil.UUIDToString(org.ID)); err != nil {
		return err
	}

	tid, err := pgutil.ParseUUID(tournamentID)
	if err != nil {
		return ErrTournamentNotFound
	}
	if _, err := s.repo.GetTournamentByID(ctx, tid, org.ID); err != nil {
		return err
	}

	rid, err := pgutil.ParseUUID(registrationID)
	if err != nil {
		return ErrRegistrationNotFound
	}

	current, err := s.repo.GetByID(ctx, rid, tid)
	if err != nil {
		return err
	}

	actorUID, err := pgutil.ParseUUID(actorID)
	if err != nil {
		return errors.New("invalid actor user id")
	}

	oldData, err := json.Marshal(registrationToResponse(current))
	if err != nil {
		return err
	}

	if err := s.repo.WithdrawWithAudit(ctx, withdrawRegistrationTxParams{
		id:             current.ID,
		tournamentID:   tid,
		orgID:          org.ID,
		actorID:        actorUID,
		oldData:        oldData,
		previousStatus: current.Status,
	}); err != nil {
		return err
	}

	// Synchronous post-commit drain.
	s.notifSvc.DrainOutbox(ctx, org.ID, s.log)

	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func assertOrgOwnership(actorOrgID, targetOrgID string) error {
	if actorOrgID == "" {
		return nil
	}
	if actorOrgID != targetOrgID {
		return ErrForbidden
	}
	return nil
}

// validateRegistrationWindow enforces Rule 3:
// registration_opens_at <= now() <= registration_closes_at
func validateRegistrationWindow(t *db.Tournament) error {
	now := time.Now()
	if t.RegistrationOpensAt.Valid && now.Before(t.RegistrationOpensAt.Time) {
		return ErrWindowNotOpen
	}
	if t.RegistrationClosesAt.Valid && now.After(t.RegistrationClosesAt.Time) {
		return ErrWindowClosed
	}
	return nil
}

func parseRegistrationStatus(s string) (db.RegistrationStatus, error) {
	st := db.RegistrationStatus(strings.ToLower(strings.TrimSpace(s)))
	switch st {
	case db.RegistrationStatusPending, db.RegistrationStatusApproved,
		db.RegistrationStatusRejected, db.RegistrationStatusWithdrawn,
		db.RegistrationStatusDisqualified:
		return st, nil
	}
	return "", ErrInvalidStatus
}

func validateStatusTransition(from, to db.RegistrationStatus) error {
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

func registrationToResponse(r *db.TournamentRegistration) *Response {
	var teamID, playerID *string
	if r.TeamID.Valid {
		s := pgutil.UUIDToString(r.TeamID)
		teamID = &s
	}
	if r.PlayerID.Valid {
		s := pgutil.UUIDToString(r.PlayerID)
		playerID = &s
	}
	var registeredBy *string
	if r.RegisteredBy.Valid {
		s := pgutil.UUIDToString(r.RegisteredBy)
		registeredBy = &s
	}
	var approvedBy *string
	if r.ApprovedBy.Valid {
		s := pgutil.UUIDToString(r.ApprovedBy)
		approvedBy = &s
	}
	var approvedAt *string
	if r.ApprovedAt.Valid {
		s := r.ApprovedAt.Time.UTC().Format(time.RFC3339)
		approvedAt = &s
	}
	return &Response{
		ID:             pgutil.UUIDToString(r.ID),
		TournamentID:   pgutil.UUIDToString(r.TournamentID),
		OrganizationID: pgutil.UUIDToString(r.OrganizationID),
		TeamID:         teamID,
		PlayerID:       playerID,
		SeedNumber:     r.SeedNumber,
		Status:         string(r.Status),
		RegisteredBy:   registeredBy,
		RegisteredAt:   r.RegisteredAt.Time.UTC().Format(time.RFC3339),
		ApprovedBy:     approvedBy,
		ApprovedAt:     approvedAt,
		Notes:          r.Notes,
		CreatedAt:      r.CreatedAt.Time.UTC().Format(time.RFC3339),
		UpdatedAt:      r.UpdatedAt.Time.UTC().Format(time.RFC3339),
	}
}
