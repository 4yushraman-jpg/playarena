package teams

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
)

var reHexColor = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
var reNonSlugChar = regexp.MustCompile(`[^a-z0-9]+`)

// Service implements team use-cases.
type Service struct {
	repo *Repository
	log  *slog.Logger
}

// NewService constructs a Service.
func NewService(repo *Repository, log *slog.Logger) *Service {
	return &Service{repo: repo, log: log}
}

// ── team CRUD ─────────────────────────────────────────────────────────────────

// Create registers a new team in the given organization.
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

	if err := validateTeamFields(req.ShortName, req.PrimaryColor, req.SecondaryColor); err != nil {
		return nil, err
	}

	baseSlug := generateTeamSlug(req.Name)
	params := db.CreateTeamParams{
		OrganizationID: org.ID,
		Name:           strings.TrimSpace(req.Name),
		ShortName:      req.ShortName,
		Description:    req.Description,
		LogoUrl:        req.LogoURL,
		HomeCity:       req.HomeCity,
		HomeVenue:      req.HomeVenue,
		FoundedYear:    req.FoundedYear,
		PrimaryColor:   req.PrimaryColor,
		SecondaryColor: req.SecondaryColor,
	}

	// Attempt slug variants until one is unique within the org (up to 10 retries).
	var team *db.Team
	for attempt := 1; attempt <= 10; attempt++ {
		if attempt == 1 {
			params.Slug = baseSlug
		} else {
			params.Slug = fmt.Sprintf("%s-%d", baseSlug, attempt)
		}
		team, err = s.repo.CreateWithAudit(ctx, createTeamTxParams{
			createParams: params,
			actorID:      actorUID,
		})
		if errors.Is(err, ErrSlugAlreadyTaken) {
			continue
		}
		break
	}
	if err != nil {
		if errors.Is(err, ErrSlugAlreadyTaken) {
			return nil, ErrSlugGenerationFailed
		}
		return nil, err
	}
	return teamToResponse(team), nil
}

// List returns a paginated page of non-disbanded teams for an organization.
// No ownership check: any authenticated user may list any org's teams.
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

	teams, err := s.repo.List(ctx, org.ID, params)
	if err != nil {
		return nil, err
	}

	total, err := s.repo.Count(ctx, org.ID, params)
	if err != nil {
		return nil, err
	}

	resp := make([]Response, len(teams))
	for i := range teams {
		resp[i] = *teamToResponse(&teams[i])
	}
	return &ListResponse{
		Teams:  resp,
		Total:  total,
		Limit:  int(params.Limit),
		Offset: int(params.Offset),
	}, nil
}

// GetByID retrieves a single team by UUID within an organization.
// No ownership check: any authenticated user may read team profiles.
//
// Disbanded teams (soft-deleted via the DELETE endpoint) are intentionally
// returned. Their status field will be "disbanded". This is by design: team
// records must remain accessible so that match winner references, tournament
// brackets, and ranking history continue to resolve to a valid record.
func (s *Service) GetByID(ctx context.Context, orgSlug, teamID string) (*Response, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	tid, err := pgutil.ParseUUID(teamID)
	if err != nil {
		return nil, ErrTeamNotFound
	}

	team, err := s.repo.GetTeamByID(ctx, tid, org.ID)
	if err != nil {
		return nil, err
	}
	return teamToResponse(team), nil
}

// Update applies a partial update to a team.
// BOLA guard: actorOrgID must match the target org or be empty (platform admin).
func (s *Service) Update(ctx context.Context, orgSlug, teamID string, req UpdateRequest, actorID, actorOrgID string) (*Response, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	if err := assertOrgOwnership(actorOrgID, pgutil.UUIDToString(org.ID)); err != nil {
		return nil, err
	}

	tid, err := pgutil.ParseUUID(teamID)
	if err != nil {
		return nil, ErrTeamNotFound
	}

	current, err := s.repo.GetTeamByID(ctx, tid, org.ID)
	if err != nil {
		return nil, err
	}

	actorUID, err := pgutil.ParseUUID(actorID)
	if err != nil {
		return nil, errors.New("invalid actor user id")
	}

	oldData, err := json.Marshal(teamToResponse(current))
	if err != nil {
		return nil, err
	}

	params := db.UpdateTeamParams{
		ID:             current.ID,
		OrganizationID: current.OrganizationID,
		Name:           current.Name,
		ShortName:      current.ShortName,
		Description:    current.Description,
		LogoUrl:        current.LogoUrl,
		HomeCity:       current.HomeCity,
		HomeVenue:      current.HomeVenue,
		FoundedYear:    current.FoundedYear,
		PrimaryColor:   current.PrimaryColor,
		SecondaryColor: current.SecondaryColor,
		Status:         current.Status,
	}

	if req.Name != nil {
		params.Name = strings.TrimSpace(*req.Name)
	}
	if req.ShortName != nil {
		if err := validateShortName(req.ShortName); err != nil {
			return nil, err
		}
		params.ShortName = req.ShortName
	}
	if req.Description != nil {
		params.Description = req.Description
	}
	if req.LogoURL != nil {
		params.LogoUrl = req.LogoURL
	}
	if req.HomeCity != nil {
		params.HomeCity = req.HomeCity
	}
	if req.HomeVenue != nil {
		params.HomeVenue = req.HomeVenue
	}
	if req.FoundedYear != nil {
		params.FoundedYear = req.FoundedYear
	}
	if req.PrimaryColor != nil {
		if err := validateColor(req.PrimaryColor); err != nil {
			return nil, err
		}
		params.PrimaryColor = req.PrimaryColor
	}
	if req.SecondaryColor != nil {
		if err := validateColor(req.SecondaryColor); err != nil {
			return nil, err
		}
		params.SecondaryColor = req.SecondaryColor
	}
	if req.Status != nil {
		st, err := parseTeamStatus(*req.Status)
		if err != nil {
			return nil, err
		}
		params.Status = st
	}

	updated, err := s.repo.UpdateWithAudit(ctx, updateTeamTxParams{
		updateParams: params,
		actorID:      actorUID,
		oldData:      oldData,
	})
	if err != nil {
		return nil, err
	}
	return teamToResponse(updated), nil
}

// Delete soft-deletes a team by setting its status to disbanded.
// The record is retained permanently for historical match and ranking data.
// BOLA guard: actorOrgID must match the target org or be empty (platform admin).
func (s *Service) Delete(ctx context.Context, orgSlug, teamID string, actorID, actorOrgID string) error {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return err
	}

	if err := assertOrgOwnership(actorOrgID, pgutil.UUIDToString(org.ID)); err != nil {
		return err
	}

	tid, err := pgutil.ParseUUID(teamID)
	if err != nil {
		return ErrTeamNotFound
	}

	current, err := s.repo.GetTeamByID(ctx, tid, org.ID)
	if err != nil {
		return err
	}

	actorUID, err := pgutil.ParseUUID(actorID)
	if err != nil {
		return errors.New("invalid actor user id")
	}

	oldData, err := json.Marshal(teamToResponse(current))
	if err != nil {
		return err
	}

	return s.repo.DisbandWithAudit(ctx, disbandTeamTxParams{
		id:      current.ID,
		orgID:   current.OrganizationID,
		actorID: actorUID,
		oldData: oldData,
	})
}

// ── team memberships ──────────────────────────────────────────────────────────

// AddMember adds a player to a team within an organization.
//
// Multi-tenant safety (validated in service BEFORE reaching the DB trigger):
//   - The team must belong to the URL org.
//   - The player must belong to the same org.
//
// Business rule: a player may not hold two simultaneous active memberships on
// the same team. Historical memberships are preserved in full.
//
// BOLA guard: actorOrgID must match the target org or be empty (platform admin).
func (s *Service) AddMember(ctx context.Context, orgSlug, teamID string, req AddMemberRequest, actorID, actorOrgID string) (*MembershipResponse, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	if err := assertOrgOwnership(actorOrgID, pgutil.UUIDToString(org.ID)); err != nil {
		return nil, err
	}

	// Resolve and verify the team belongs to the URL org.
	tid, err := pgutil.ParseUUID(teamID)
	if err != nil {
		return nil, ErrTeamNotFound
	}
	if _, err := s.repo.GetTeamByID(ctx, tid, org.ID); err != nil {
		return nil, err
	}

	// Verify the player belongs to the same org.
	pid, err := pgutil.ParseUUID(req.PlayerID)
	if err != nil {
		return nil, ErrPlayerNotFound
	}
	if err := s.repo.GetPlayerByID(ctx, pid, org.ID); err != nil {
		if errors.Is(err, ErrPlayerNotFound) {
			return nil, ErrCrossOrgMembership
		}
		return nil, err
	}

	// Reject duplicate active memberships.
	existing, err := s.repo.GetActiveMembership(ctx, tid, pid, org.ID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrMembershipAlreadyActive
	}

	actorUID, err := pgutil.ParseUUID(actorID)
	if err != nil {
		return nil, errors.New("invalid actor user id")
	}

	role, err := parseMembershipRole(req.Role)
	if err != nil {
		return nil, err
	}

	m, err := s.repo.AddMemberWithAudit(ctx, addMemberTxParams{
		createParams: db.CreateMembershipParams{
			TeamID:         tid,
			PlayerID:       pid,
			OrganizationID: org.ID,
			Role:           role,
			JerseyNumber:   req.JerseyNumber,
			Notes:          req.Notes,
		},
		actorID: actorUID,
	})
	if err != nil {
		return nil, err
	}
	return membershipToResponse(m), nil
}

// ListMembers returns all currently active members of a team.
// No ownership check: any authenticated user may read team rosters.
func (s *Service) ListMembers(ctx context.Context, orgSlug, teamID string) (*MemberListResponse, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	tid, err := pgutil.ParseUUID(teamID)
	if err != nil {
		return nil, ErrTeamNotFound
	}
	if _, err := s.repo.GetTeamByID(ctx, tid, org.ID); err != nil {
		return nil, err
	}

	members, err := s.repo.ListActiveMembers(ctx, tid, org.ID)
	if err != nil {
		return nil, err
	}

	resp := make([]MembershipResponse, len(members))
	for i := range members {
		resp[i] = *membershipToResponse(&members[i])
	}
	return &MemberListResponse{Members: resp}, nil
}

// RemoveMember soft-removes a team membership by setting status=released and
// left_at=NOW(). Historical rows are preserved.
// BOLA guard: actorOrgID must match the target org or be empty (platform admin).
func (s *Service) RemoveMember(ctx context.Context, orgSlug, teamID, membershipID string, actorID, actorOrgID string) error {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return err
	}

	if err := assertOrgOwnership(actorOrgID, pgutil.UUIDToString(org.ID)); err != nil {
		return err
	}

	tid, err := pgutil.ParseUUID(teamID)
	if err != nil {
		return ErrTeamNotFound
	}
	if _, err := s.repo.GetTeamByID(ctx, tid, org.ID); err != nil {
		return err
	}

	mid, err := pgutil.ParseUUID(membershipID)
	if err != nil {
		return ErrMembershipNotFound
	}

	// Fetch pre-removal snapshot for audit log.
	current, err := s.repo.GetMembershipByID(ctx, mid, org.ID)
	if err != nil {
		return err
	}
	// Verify the membership belongs to the URL team.
	if pgutil.UUIDToString(current.TeamID) != pgutil.UUIDToString(tid) {
		return ErrMembershipNotFound
	}

	actorUID, err := pgutil.ParseUUID(actorID)
	if err != nil {
		return errors.New("invalid actor user id")
	}

	oldData, err := json.Marshal(membershipToResponse(current))
	if err != nil {
		return err
	}

	return s.repo.RemoveMemberWithAudit(ctx, removeMemberTxParams{
		id:      current.ID,
		teamID:  tid,
		orgID:   org.ID,
		actorID: actorUID,
		oldData: oldData,
	})
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

func generateTeamSlug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = reNonSlugChar.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 90 {
		s = s[:90]
		s = strings.TrimRight(s, "-")
	}
	if len(s) < 3 {
		s = "team-" + s
	}
	if len(s) < 3 {
		s = "team-x"
	}
	return s
}

func validateTeamFields(shortName, primaryColor, secondaryColor *string) error {
	if err := validateShortName(shortName); err != nil {
		return err
	}
	if err := validateColor(primaryColor); err != nil {
		return err
	}
	return validateColor(secondaryColor)
}

func validateShortName(s *string) error {
	if s == nil || *s == "" {
		return nil
	}
	n := len([]rune(strings.TrimSpace(*s)))
	if n < 2 || n > 10 {
		return ErrInvalidShortName
	}
	return nil
}

func validateColor(s *string) error {
	if s == nil || *s == "" {
		return nil
	}
	if !reHexColor.MatchString(*s) {
		return ErrInvalidColor
	}
	return nil
}

func parseTeamStatus(s string) (db.TeamStatus, error) {
	st := db.TeamStatus(strings.ToLower(strings.TrimSpace(s)))
	switch st {
	case db.TeamStatusActive, db.TeamStatusInactive, db.TeamStatusDisbanded:
		return st, nil
	}
	return "", ErrInvalidStatus
}

func parseMembershipRole(s *string) (db.MembershipRole, error) {
	if s == nil || *s == "" {
		return db.MembershipRolePlayer, nil
	}
	r := db.MembershipRole(strings.ToLower(strings.TrimSpace(*s)))
	switch r {
	case db.MembershipRolePlayer, db.MembershipRoleCaptain, db.MembershipRoleViceCaptain,
		db.MembershipRoleCoach, db.MembershipRoleManager, db.MembershipRoleSupportStaff:
		return r, nil
	}
	return "", ErrInvalidMembershipRole
}

func teamToResponse(t *db.Team) *Response {
	return &Response{
		ID:             pgutil.UUIDToString(t.ID),
		OrganizationID: pgutil.UUIDToString(t.OrganizationID),
		Name:           t.Name,
		ShortName:      t.ShortName,
		Slug:           t.Slug,
		Description:    t.Description,
		LogoURL:        t.LogoUrl,
		HomeCity:       t.HomeCity,
		HomeVenue:      t.HomeVenue,
		FoundedYear:    t.FoundedYear,
		PrimaryColor:   t.PrimaryColor,
		SecondaryColor: t.SecondaryColor,
		Status:         string(t.Status),
		CreatedAt:      t.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt:      t.UpdatedAt.Time.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

func membershipToResponse(m *db.TeamMembership) *MembershipResponse {
	var leftAt *string
	if m.LeftAt.Valid {
		s := m.LeftAt.Time.UTC().Format(time.RFC3339)
		leftAt = &s
	}
	return &MembershipResponse{
		ID:             pgutil.UUIDToString(m.ID),
		TeamID:         pgutil.UUIDToString(m.TeamID),
		PlayerID:       pgutil.UUIDToString(m.PlayerID),
		OrganizationID: pgutil.UUIDToString(m.OrganizationID),
		Role:           string(m.Role),
		JerseyNumber:   m.JerseyNumber,
		Status:         string(m.Status),
		JoinedAt:       m.JoinedAt.Time.UTC().Format(time.RFC3339),
		LeftAt:         leftAt,
		Notes:          m.Notes,
		CreatedAt:      m.CreatedAt.Time.UTC().Format(time.RFC3339),
		UpdatedAt:      m.UpdatedAt.Time.UTC().Format(time.RFC3339),
	}
}
