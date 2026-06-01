package auth

import (
	"context"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
)

// AuthorizationService performs database-backed authorization checks.
// It is intentionally separate from Service (which handles session management)
// so that permission checks can be wired into any module without pulling in the
// full auth service dependency tree.
//
// All methods are safe to call concurrently.
type AuthorizationService struct {
	queries *db.Queries
}

// NewAuthorizationService constructs an AuthorizationService backed by the
// given query wrapper.
func NewAuthorizationService(queries *db.Queries) *AuthorizationService {
	return &AuthorizationService{queries: queries}
}

// HasRole reports whether the authenticated user holds at least one of the
// given role slugs in the specified organization context.
//
// orgID is the string UUID of the organization. Pass an empty string to check
// only platform-level grants (user_organization_roles.organization_id IS NULL).
//
// Returns (false, nil) when the user ID or org ID is not a valid UUID.
func (s *AuthorizationService) HasRole(ctx context.Context, userID, orgID string, roleSlugs ...string) (bool, error) {
	uid, err := pgutil.ParseUUID(userID)
	if err != nil {
		return false, nil
	}

	oid := pgutil.ParseOptionalUUID(orgID)

	roles, err := s.queries.GetUserRoles(ctx, db.GetUserRolesParams{
		UserID:         uid,
		OrganizationID: oid,
	})
	if err != nil {
		return false, err
	}

	wanted := make(map[string]struct{}, len(roleSlugs))
	for _, slug := range roleSlugs {
		wanted[slug] = struct{}{}
	}

	for _, r := range roles {
		if _, ok := wanted[r.Slug]; ok {
			return true, nil
		}
	}
	return false, nil
}

// HasPermission reports whether the authenticated user holds the given
// permission slug in the specified organization context.
//
// orgID is the string UUID of the organization. Pass an empty string to
// evaluate only platform-level grants.
//
// Uses a single EXISTS query: no N+1, short-circuits on first matching grant.
//
// Returns (false, nil) when userID or orgID is not a valid UUID.
func (s *AuthorizationService) HasPermission(ctx context.Context, userID, orgID, permSlug string) (bool, error) {
	uid, err := pgutil.ParseUUID(userID)
	if err != nil {
		return false, nil
	}

	oid := pgutil.ParseOptionalUUID(orgID)

	return s.queries.HasPermission(ctx, db.HasPermissionParams{
		UserID:         uid,
		OrganizationID: oid,
		Slug:           permSlug,
	})
}

// GetPermissions returns the full list of permission slugs held by the user
// in the given organization context.
func (s *AuthorizationService) GetPermissions(ctx context.Context, userID, orgID string) ([]string, error) {
	uid, err := pgutil.ParseUUID(userID)
	if err != nil {
		return nil, nil
	}

	oid := pgutil.ParseOptionalUUID(orgID)

	perms, err := s.queries.GetUserPermissions(ctx, db.GetUserPermissionsParams{
		UserID:         uid,
		OrganizationID: oid,
	})
	if err != nil {
		return nil, err
	}

	slugs := make([]string, len(perms))
	for i, p := range perms {
		slugs[i] = p.Slug
	}
	return slugs, nil
}
