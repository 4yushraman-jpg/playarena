package players

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
)

// Service implements player use-cases.
type Service struct {
	repo *Repository
	log  *slog.Logger
}

// NewService constructs a Service.
func NewService(repo *Repository, log *slog.Logger) *Service {
	return &Service{repo: repo, log: log}
}

// ── public methods ────────────────────────────────────────────────────────────

// Create registers a new player in the given organization.
//
// BOLA guard: actorOrgID must match the target org or be empty (platform admin).
func (s *Service) Create(ctx context.Context, orgSlug string, req CreateRequest, actorID, actorOrgID string) (*Response, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	if err := assertOrgOwnership(actorOrgID, pgutil.UUIDToString(org.ID)); err != nil {
		return nil, err
	}

	actorUID, err := pgutil.ParseUUID(actorID)
	if err != nil {
		return nil, errors.New("invalid actor user id")
	}

	if err := validateDominantHand(req.DominantHand); err != nil {
		return nil, err
	}

	nationality, err := normalizeNationality(req.Nationality)
	if err != nil {
		return nil, err
	}

	dob, err := parseDateOfBirth(req.DateOfBirth)
	if err != nil {
		return nil, err
	}

	userUID, err := parseOptionalUserID(req.UserID)
	if err != nil {
		return nil, err
	}

	params := db.CreatePlayerParams{
		OrganizationID: org.ID,
		UserID:         userUID,
		DisplayName:    strings.TrimSpace(req.DisplayName),
		JerseyNumber:   req.JerseyNumber,
		Position:       req.Position,
		HeightCm:       req.HeightCm,
		WeightKg:       req.WeightKg,
		DominantHand:   req.DominantHand,
		Nationality:    nationality,
		DateOfBirth:    dob,
		Bio:            req.Bio,
	}

	player, err := s.repo.CreateWithAudit(ctx, createPlayerTxParams{
		createParams: params,
		actorID:      actorUID,
	})
	if err != nil {
		return nil, err
	}
	return playerToResponse(player), nil
}

// List returns a paginated page of non-inactive players for an organization.
// No ownership check: any authenticated user may list any org's players.
//
// avatar_url is intentionally omitted from list responses. The list view renders
// thumbnails at size-sm (8 × 8px) using initials only, so media URLs add no
// visible value. Full media is populated by GetByID. Do NOT add GetPrimaryMediaURL
// here without batching it across all rows; a per-row media lookup is an N+1 query.
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

	players, err := s.repo.List(ctx, org.ID, params)
	if err != nil {
		return nil, err
	}

	total, err := s.repo.Count(ctx, org.ID, params)
	if err != nil {
		return nil, err
	}

	resp := make([]Response, len(players))
	for i := range players {
		resp[i] = *playerToResponse(&players[i])
	}
	return &ListResponse{
		Players: resp,
		Total:   total,
		Limit:   int(params.Limit),
		Offset:  int(params.Offset),
	}, nil
}

// GetByID retrieves a single player by UUID within an organization.
// No ownership check: any authenticated user may read player profiles.
//
// Inactive players (soft-deleted via the DELETE endpoint) are intentionally
// returned. Their status field will be "inactive". This is by design: player
// records must remain retrievable indefinitely so that historical team
// memberships, match event references, and audit logs continue to resolve to
// a valid record. Callers that need to distinguish active from deleted players
// should inspect the Status field of the returned Response.
func (s *Service) GetByID(ctx context.Context, orgSlug, playerID string) (*Response, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	pid, err := pgutil.ParseUUID(playerID)
	if err != nil {
		return nil, ErrPlayerNotFound
	}

	player, err := s.repo.GetByID(ctx, pid, org.ID)
	if err != nil {
		return nil, err
	}
	resp := playerToResponse(player)
	avatarURL, err := s.repo.GetPrimaryMediaURL(ctx, player.ID, org.ID)
	if err != nil {
		return nil, err
	}
	resp.AvatarURL = avatarURL
	return resp, nil
}

// Update applies a partial update to a player.
//
// BOLA guard: actorOrgID must match the target org or be empty (platform admin).
func (s *Service) Update(ctx context.Context, orgSlug, playerID string, req UpdateRequest, actorID, actorOrgID string) (*Response, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	if err := assertOrgOwnership(actorOrgID, pgutil.UUIDToString(org.ID)); err != nil {
		return nil, err
	}

	pid, err := pgutil.ParseUUID(playerID)
	if err != nil {
		return nil, ErrPlayerNotFound
	}

	current, err := s.repo.GetByID(ctx, pid, org.ID)
	if err != nil {
		return nil, err
	}

	actorUID, err := pgutil.ParseUUID(actorID)
	if err != nil {
		return nil, errors.New("invalid actor user id")
	}

	// Capture old state for the audit record BEFORE applying changes.
	oldData, err := json.Marshal(playerToResponse(current))
	if err != nil {
		return nil, err
	}

	// Apply non-nil request fields over the current state.
	params := db.UpdatePlayerParams{
		ID:             current.ID,
		OrganizationID: current.OrganizationID,
		DisplayName:    current.DisplayName,
		JerseyNumber:   current.JerseyNumber,
		Position:       current.Position,
		HeightCm:       current.HeightCm,
		WeightKg:       current.WeightKg,
		DominantHand:   current.DominantHand,
		Nationality:    current.Nationality,
		DateOfBirth:    current.DateOfBirth,
		Bio:            current.Bio,
		Status:         current.Status,
	}

	if req.DisplayName != nil {
		params.DisplayName = strings.TrimSpace(*req.DisplayName)
	}
	if req.JerseyNumber != nil {
		params.JerseyNumber = req.JerseyNumber
	}
	if req.Position != nil {
		params.Position = req.Position
	}
	if req.HeightCm != nil {
		params.HeightCm = req.HeightCm
	}
	if req.WeightKg != nil {
		params.WeightKg = req.WeightKg
	}
	if req.DominantHand != nil {
		if err := validateDominantHand(req.DominantHand); err != nil {
			return nil, err
		}
		params.DominantHand = req.DominantHand
	}
	if req.Nationality != nil {
		nat, err := normalizeNationality(req.Nationality)
		if err != nil {
			return nil, err
		}
		params.Nationality = nat
	}
	if req.DateOfBirth != nil {
		dob, err := parseDateOfBirth(req.DateOfBirth)
		if err != nil {
			return nil, err
		}
		params.DateOfBirth = dob
	}
	if req.Bio != nil {
		params.Bio = req.Bio
	}
	if req.Status != nil {
		st, err := parsePlayerStatus(*req.Status)
		if err != nil {
			return nil, err
		}
		params.Status = st
	}

	updated, err := s.repo.UpdateWithAudit(ctx, updatePlayerTxParams{
		updateParams: params,
		actorID:      actorUID,
		oldData:      oldData,
	})
	if err != nil {
		return nil, err
	}
	return playerToResponse(updated), nil
}

// Delete soft-deletes a player by setting their status to inactive.
// The record is retained permanently for historical match and roster data.
//
// BOLA guard: actorOrgID must match the target org or be empty (platform admin).
func (s *Service) Delete(ctx context.Context, orgSlug, playerID string, actorID, actorOrgID string) error {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return err
	}

	if err := assertOrgOwnership(actorOrgID, pgutil.UUIDToString(org.ID)); err != nil {
		return err
	}

	pid, err := pgutil.ParseUUID(playerID)
	if err != nil {
		return ErrPlayerNotFound
	}

	current, err := s.repo.GetByID(ctx, pid, org.ID)
	if err != nil {
		return err
	}

	actorUID, err := pgutil.ParseUUID(actorID)
	if err != nil {
		return errors.New("invalid actor user id")
	}

	oldData, err := json.Marshal(playerToResponse(current))
	if err != nil {
		return err
	}

	return s.repo.SoftDeleteWithAudit(ctx, softDeletePlayerTxParams{
		id:      current.ID,
		orgID:   current.OrganizationID,
		actorID: actorUID,
		oldData: oldData,
	})
}

// ── helpers ───────────────────────────────────────────────────────────────────

// assertOrgOwnership is the centralised BOLA guard for the players domain.
// Platform admins (actorOrgID == "") are unconditionally permitted.
func assertOrgOwnership(actorOrgID, targetOrgID string) error {
	if actorOrgID == "" {
		return nil
	}
	if actorOrgID != targetOrgID {
		return ErrForbidden
	}
	return nil
}

func validateDominantHand(s *string) error {
	if s == nil || *s == "" {
		return nil
	}
	switch strings.ToLower(*s) {
	case "left", "right", "ambidextrous":
		return nil
	}
	return ErrInvalidDominantHand
}

func normalizeNationality(s *string) (*string, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	code := strings.ToUpper(strings.TrimSpace(*s))
	if len(code) != 2 {
		return nil, ErrInvalidNationality
	}
	return &code, nil
}

func parseDateOfBirth(s *string) (pgtype.Date, error) {
	if s == nil || *s == "" {
		return pgtype.Date{}, nil
	}
	t, err := time.Parse("2006-01-02", *s)
	if err != nil || !t.Before(time.Now()) {
		return pgtype.Date{}, ErrInvalidDateOfBirth
	}
	return pgtype.Date{Time: t, Valid: true}, nil
}

func parseOptionalUserID(s *string) (pgtype.UUID, error) {
	if s == nil || *s == "" {
		return pgtype.UUID{}, nil
	}
	return pgutil.ParseUUID(*s)
}

func parsePlayerStatus(s string) (db.PlayerStatus, error) {
	st := db.PlayerStatus(strings.ToLower(strings.TrimSpace(s)))
	switch st {
	case db.PlayerStatusActive, db.PlayerStatusInactive,
		db.PlayerStatusInjured, db.PlayerStatusSuspended, db.PlayerStatusRetired:
		return st, nil
	}
	return "", ErrInvalidStatus
}

// playerToResponse converts a db.Player into the HTTP Response representation.
func playerToResponse(p *db.Player) *Response {
	var userID *string
	if p.UserID.Valid {
		uid := pgutil.UUIDToString(p.UserID)
		userID = &uid
	}
	var dob *string
	if p.DateOfBirth.Valid {
		s := p.DateOfBirth.Time.Format("2006-01-02")
		dob = &s
	}
	return &Response{
		ID:             pgutil.UUIDToString(p.ID),
		OrganizationID: pgutil.UUIDToString(p.OrganizationID),
		UserID:         userID,
		DisplayName:    p.DisplayName,
		JerseyNumber:   p.JerseyNumber,
		Position:       p.Position,
		HeightCm:       p.HeightCm,
		WeightKg:       p.WeightKg,
		DominantHand:   p.DominantHand,
		Nationality:    p.Nationality,
		DateOfBirth:    dob,
		Status:         string(p.Status),
		Bio:            p.Bio,
		Visibility:     p.Visibility,
		CreatedAt:      p.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt:      p.UpdatedAt.Time.UTC().Format("2006-01-02T15:04:05Z"),
	}
}
