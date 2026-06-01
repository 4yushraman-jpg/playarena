package organizations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
)

// Service implements organization use-cases.
type Service struct {
	repo *Repository
	log  *slog.Logger
}

// NewService constructs a Service.
func NewService(repo *Repository, log *slog.Logger) *Service {
	return &Service{repo: repo, log: log}
}

// ── public methods ────────────────────────────────────────────────────────────

// Create creates a new organization and grants the caller the org_owner role.
// Slug is auto-generated from the name and guaranteed unique within the DB.
func (s *Service) Create(ctx context.Context, req CreateRequest, actorID string) (*Response, error) {
	orgType, err := parseOrgType(req.Type)
	if err != nil {
		return nil, err
	}

	country, err := normalizeCountry(req.Country)
	if err != nil {
		return nil, err
	}

	creatorUID, err := pgutil.ParseUUID(actorID)
	if err != nil {
		return nil, errors.New("invalid actor user id")
	}

	baseSlug := generateSlug(req.Name)

	orgParams := db.CreateOrganizationParams{
		Name:        strings.TrimSpace(req.Name),
		Description: req.Description,
		Type:        orgType,
		Website:     req.Website,
		Email:       req.Email,
		Phone:       req.Phone,
		Country:     country,
		City:        req.City,
	}

	// Attempt slug variants until one is unique (up to 10 retries).
	var org *db.Organization
	for attempt := 1; attempt <= 10; attempt++ {
		if attempt == 1 {
			orgParams.Slug = baseSlug
		} else {
			orgParams.Slug = fmt.Sprintf("%s-%d", baseSlug, attempt)
		}

		org, err = s.repo.CreateWithOwnerGrant(ctx, createOrgTxParams{
			orgParams: orgParams,
			creatorID: creatorUID,
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

	return orgToResponse(org), nil
}

// List returns a page of organizations ordered by creation time (newest first).
// Limit and offset are validated and capped before hitting the DB.
func (s *Service) List(ctx context.Context, params ListParams) (*ListResponse, error) {
	// Enforce bounds so the DB is never asked for an unreasonable page.
	if params.Limit <= 0 || params.Limit > MaxListLimit {
		params.Limit = DefaultListLimit
	}
	if params.Offset < 0 {
		params.Offset = 0
	}

	orgs, err := s.repo.List(ctx, params)
	if err != nil {
		return nil, err
	}

	resp := make([]Response, len(orgs))
	for i := range orgs {
		resp[i] = *orgToResponse(&orgs[i])
	}
	return &ListResponse{
		Organizations: resp,
		Total:         len(resp),
		Limit:         int(params.Limit),
		Offset:        int(params.Offset),
	}, nil
}

// GetBySlug retrieves an organization by its URL slug.
func (s *Service) GetBySlug(ctx context.Context, slug string) (*Response, error) {
	org, err := s.repo.GetBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	return orgToResponse(org), nil
}

// Update applies partial updates to an organization identified by slug.
//
// C1 (BOLA) fix: the actorOrgID parameter is the organization_id embedded in
// the caller's JWT. Before applying any changes the service verifies that the
// target organization's ID matches the actor's org context. Platform admins
// (actorOrgID == "") are exempt and may update any organization.
func (s *Service) Update(ctx context.Context, slug string, req UpdateRequest, actorID, actorOrgID string) (*Response, error) {
	current, err := s.repo.GetBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}

	// C1 fix: reject if the actor is acting in a different org context.
	if err := assertOrgOwnership(actorOrgID, pgutil.UUIDToString(current.ID)); err != nil {
		return nil, err
	}

	actorUID, err := pgutil.ParseUUID(actorID)
	if err != nil {
		return nil, errors.New("invalid actor user id")
	}

	// Capture old state for the audit record BEFORE applying changes.
	oldData, err := json.Marshal(orgToResponse(current))
	if err != nil {
		return nil, err
	}

	// Apply non-nil request fields over the current state.
	params := db.UpdateOrganizationParams{
		ID:          current.ID,
		Name:        current.Name,
		Description: current.Description,
		Type:        current.Type,
		Website:     current.Website,
		Email:       current.Email,
		Phone:       current.Phone,
		Country:     current.Country,
		City:        current.City,
	}

	if req.Name != nil {
		params.Name = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		params.Description = req.Description
	}
	if req.Type != nil {
		t, err := parseOrgType(*req.Type)
		if err != nil {
			return nil, err
		}
		params.Type = t
	}
	if req.Website != nil {
		params.Website = req.Website
	}
	if req.Email != nil {
		params.Email = req.Email
	}
	if req.Phone != nil {
		params.Phone = req.Phone
	}
	if req.Country != nil {
		country, err := normalizeCountry(req.Country)
		if err != nil {
			return nil, err
		}
		params.Country = country
	}
	if req.City != nil {
		params.City = req.City
	}

	updated, err := s.repo.UpdateWithAudit(ctx, updateOrgTxParams{
		updateParams: params,
		actorID:      actorUID,
		oldData:      oldData,
	})
	if err != nil {
		return nil, err
	}
	return orgToResponse(updated), nil
}

// Delete permanently removes an organization. All child records are cascade-deleted.
//
// C1 (BOLA) fix: same ownership check as Update.
func (s *Service) Delete(ctx context.Context, slug string, actorID, actorOrgID string) error {
	org, err := s.repo.GetBySlug(ctx, slug)
	if err != nil {
		return err
	}

	// C1 fix.
	if err := assertOrgOwnership(actorOrgID, pgutil.UUIDToString(org.ID)); err != nil {
		return err
	}

	actorUID, err := pgutil.ParseUUID(actorID)
	if err != nil {
		return errors.New("invalid actor user id")
	}

	oldData, err := json.Marshal(orgToResponse(org))
	if err != nil {
		return err
	}

	return s.repo.DeleteWithAudit(ctx, deleteOrgTxParams{
		orgID:   org.ID,
		actorID: actorUID,
		oldData: oldData,
	})
}

// ── helpers ───────────────────────────────────────────────────────────────────

var reNonSlugChar = regexp.MustCompile(`[^a-z0-9]+`)

func generateSlug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = reNonSlugChar.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")

	if len(s) > 90 {
		s = s[:90]
		s = strings.TrimRight(s, "-")
	}
	if len(s) < 3 {
		s = "org-" + s
	}
	if len(s) < 3 {
		s = "org-x"
	}
	return s
}

func parseOrgType(s string) (db.OrgType, error) {
	t := db.OrgType(strings.ToLower(strings.TrimSpace(s)))
	switch t {
	case db.OrgTypeClub, db.OrgTypeFederation, db.OrgTypeSchool, db.OrgTypeCorporate, db.OrgTypeIndependent:
		return t, nil
	}
	return "", ErrInvalidOrgType
}

func normalizeCountry(s *string) (*string, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	code := strings.ToUpper(strings.TrimSpace(*s))
	if len(code) != 2 {
		return nil, ErrInvalidCountryCode
	}
	return &code, nil
}

// orgToResponse converts a db.Organization into the HTTP Response representation.
func orgToResponse(o *db.Organization) *Response {
	return &Response{
		ID:          pgutil.UUIDToString(o.ID),
		Name:        o.Name,
		Slug:        o.Slug,
		Type:        string(o.Type),
		Status:      string(o.Status),
		Description: o.Description,
		Website:     o.Website,
		Email:       o.Email,
		Phone:       o.Phone,
		Country:     o.Country,
		City:        o.City,
		CreatedAt:   o.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   o.UpdatedAt.Time.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

// assertOrgOwnership validates that the actor is allowed to modify the target
// organization. It is the centralised guard for the C1 / BOLA fix.
//
// Platform admins pass actorOrgID == "" (their JWT carries no org context) and
// are unconditionally allowed. All other users must have an actorOrgID that
// matches the target org's ID.
func assertOrgOwnership(actorOrgID, targetOrgID string) error {
	if actorOrgID == "" {
		// Platform admin — permitted on any org.
		return nil
	}
	if actorOrgID != targetOrgID {
		return ErrForbidden
	}
	return nil
}
