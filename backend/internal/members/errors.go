package members

import "errors"

var (
	ErrOrganizationNotFound = errors.New("organization not found")
	ErrUserNotFound         = errors.New("user not found")
	ErrRoleNotFound         = errors.New("role not found")
	ErrGrantNotFound        = errors.New("role grant not found")

	// ErrForbidden is returned when the caller's org context does not match the
	// target organization (BOLA / IDOR prevention).
	ErrForbidden = errors.New("access denied: you do not have permission to manage this organization's members")

	// ErrLastOwner prevents removing the last org_owner grant, which would
	// leave the organization without an owner and unmanageable.
	ErrLastOwner = errors.New("cannot revoke the last org_owner role; assign another owner first")

	// ErrInvalidExpiresAt is returned when expires_at is provided but not a valid RFC3339 timestamp.
	ErrInvalidExpiresAt = errors.New("expires_at must be a valid RFC3339 timestamp in the future")
)
