package members

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
)

// Service implements member management use-cases.
type Service struct {
	repo *Repository
	log  *slog.Logger
}

// NewService constructs a Service.
func NewService(repo *Repository, log *slog.Logger) *Service {
	return &Service{repo: repo, log: log}
}

// ── public methods ────────────────────────────────────────────────────────────

// ListMembers returns all members with their active role grants in the org.
func (s *Service) ListMembers(ctx context.Context, orgSlug, actorOrgID string) (*ListResponse, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	if err := assertOrgOwnership(actorOrgID, pgutil.UUIDToString(org.ID)); err != nil {
		return nil, err
	}

	rows, err := s.repo.ListOrgMembersWithRoles(ctx, org.ID)
	if err != nil {
		return nil, err
	}

	return &ListResponse{Members: aggregateMembers(rows)}, nil
}

// GetMember returns all active role grants for one user in the org.
func (s *Service) GetMember(ctx context.Context, orgSlug, targetUserID, actorOrgID string) (*MemberResponse, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	if err := assertOrgOwnership(actorOrgID, pgutil.UUIDToString(org.ID)); err != nil {
		return nil, err
	}

	uid, err := pgutil.ParseUUID(targetUserID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	user, err := s.repo.GetUserByID(ctx, uid)
	if err != nil {
		return nil, err
	}

	grants, err := s.repo.GetUserGrantsInOrg(ctx, uid, org.ID)
	if err != nil {
		return nil, err
	}

	return buildMemberResponse(user, grants), nil
}

// GrantRole assigns a role to a user within an org.
func (s *Service) GrantRole(ctx context.Context, orgSlug, targetUserID string, req GrantRequest, actorID, actorOrgID string) (*MemberResponse, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	if err := assertOrgOwnership(actorOrgID, pgutil.UUIDToString(org.ID)); err != nil {
		return nil, err
	}

	uid, err := pgutil.ParseUUID(targetUserID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	user, err := s.repo.GetUserByID(ctx, uid)
	if err != nil {
		return nil, err
	}

	role, err := s.repo.GetRoleBySlug(ctx, req.RoleSlug)
	if err != nil {
		return nil, err
	}

	actorUID, err := pgutil.ParseUUID(actorID)
	if err != nil {
		return nil, errors.New("invalid actor user id")
	}

	expiresAt, err := parseExpiresAt(req.ExpiresAt)
	if err != nil {
		return nil, err
	}

	if err := s.repo.GrantRoleWithAudit(ctx, grantRoleTxParams{
		grantParams: db.GrantOrgRoleParams{
			UserID:         uid,
			OrganizationID: org.ID,
			RoleID:         role.ID,
			GrantedBy:      pgtype.UUID{Bytes: actorUID.Bytes, Valid: true},
			ExpiresAt:      expiresAt,
		},
		actorID: actorUID,
	}); err != nil {
		return nil, err
	}

	grants, err := s.repo.GetUserGrantsInOrg(ctx, uid, org.ID)
	if err != nil {
		return nil, err
	}

	return buildMemberResponse(user, grants), nil
}

// RevokeRole removes a role from a user within an org.
// Returns ErrLastOwner when the revocation would remove the last org_owner.
func (s *Service) RevokeRole(ctx context.Context, orgSlug, targetUserID, roleSlug, actorID, actorOrgID string) error {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return err
	}

	if err := assertOrgOwnership(actorOrgID, pgutil.UUIDToString(org.ID)); err != nil {
		return err
	}

	uid, err := pgutil.ParseUUID(targetUserID)
	if err != nil {
		return ErrUserNotFound
	}

	if _, err := s.repo.GetUserByID(ctx, uid); err != nil {
		return err
	}

	if roleSlug == "org_owner" {
		count, err := s.repo.CountActiveOrgOwners(ctx, org.ID)
		if err != nil {
			return err
		}
		if count <= 1 {
			return ErrLastOwner
		}
	}

	actorUID, err := pgutil.ParseUUID(actorID)
	if err != nil {
		return errors.New("invalid actor user id")
	}

	return s.repo.RevokeRoleWithAudit(ctx, revokeRoleTxParams{
		revokeParams: db.RevokeRoleFromUserInOrgParams{
			UserID:   uid,
			OrgID:    org.ID,
			RoleSlug: roleSlug,
		},
		orgID:   org.ID,
		actorID: actorUID,
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

func parseExpiresAt(s *string) (pgtype.Timestamptz, error) {
	if s == nil || *s == "" {
		return pgtype.Timestamptz{}, nil
	}
	t, err := time.Parse(time.RFC3339, *s)
	if err != nil || !t.After(time.Now()) {
		return pgtype.Timestamptz{}, ErrInvalidExpiresAt
	}
	return pgtype.Timestamptz{Time: t, Valid: true}, nil
}

func formatTimestamp(ts pgtype.Timestamptz) string {
	if !ts.Valid {
		return ""
	}
	return ts.Time.UTC().Format(time.RFC3339)
}

func formatOptionalUUID(uid pgtype.UUID) *string {
	if !uid.Valid {
		return nil
	}
	s := pgutil.UUIDToString(uid)
	return &s
}

func grantFromRow(row db.GetUserGrantsInOrgRow) RoleGrant {
	g := RoleGrant{
		GrantID:   pgutil.UUIDToString(row.GrantID),
		RoleSlug:  row.RoleSlug,
		RoleName:  row.RoleName,
		GrantedAt: formatTimestamp(row.GrantedAt),
		GrantedBy: formatOptionalUUID(row.GrantedBy),
	}
	if row.ExpiresAt.Valid {
		s := row.ExpiresAt.Time.UTC().Format(time.RFC3339)
		g.ExpiresAt = &s
	}
	return g
}

func buildMemberResponse(user *db.User, grants []db.GetUserGrantsInOrgRow) *MemberResponse {
	roles := make([]RoleGrant, 0, len(grants))
	for _, g := range grants {
		roles = append(roles, grantFromRow(g))
	}
	return &MemberResponse{
		UserID:     pgutil.UUIDToString(user.ID),
		Email:      user.Email,
		Username:   user.Username,
		FirstName:  user.FirstName,
		LastName:   user.LastName,
		UserStatus: string(user.Status),
		Roles:      roles,
	}
}

// aggregateMembers groups the flat (user × role) rows into per-user responses.
func aggregateMembers(rows []db.ListOrgMembersWithRolesRow) []MemberResponse {
	// Preserve insertion order while deduplicating by user_id.
	order := make([]string, 0)
	byUser := make(map[string]*MemberResponse)

	for _, row := range rows {
		uid := pgutil.UUIDToString(row.UserID)
		if _, exists := byUser[uid]; !exists {
			order = append(order, uid)
			byUser[uid] = &MemberResponse{
				UserID:     uid,
				Email:      row.Email,
				Username:   row.Username,
				FirstName:  row.FirstName,
				LastName:   row.LastName,
				UserStatus: string(row.UserStatus),
				Roles:      []RoleGrant{},
			}
		}

		g := RoleGrant{
			GrantID:   pgutil.UUIDToString(row.GrantID),
			RoleSlug:  row.RoleSlug,
			RoleName:  row.RoleName,
			GrantedAt: formatTimestamp(row.GrantedAt),
			GrantedBy: formatOptionalUUID(row.GrantedBy),
		}
		if row.ExpiresAt.Valid {
			s := row.ExpiresAt.Time.UTC().Format(time.RFC3339)
			g.ExpiresAt = &s
		}
		byUser[uid].Roles = append(byUser[uid].Roles, g)
	}

	result := make([]MemberResponse, 0, len(order))
	for _, uid := range order {
		result = append(result, *byUser[uid])
	}
	return result
}
