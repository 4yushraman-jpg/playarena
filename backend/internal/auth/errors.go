package auth

import (
	"errors"
	"fmt"
)

// Sentinel errors — service layer returns these; handlers map them to HTTP codes.
var (
	// Credential errors
	ErrInvalidCredentials = errors.New("invalid email or password")

	// User state errors
	ErrUserSuspended           = errors.New("user account is suspended")
	ErrUserInactive            = errors.New("user account is inactive")
	ErrUserPendingVerification = errors.New("email address has not been verified")
	ErrUserNotFound            = errors.New("user not found")

	// Registration errors
	ErrEmailAlreadyRegistered = errors.New("email address is already registered")
	ErrUsernameAlreadyTaken   = errors.New("username is already taken")

	// Token errors
	ErrInvalidToken     = errors.New("invalid token")
	ErrExpiredToken     = errors.New("token has expired")
	ErrRevokedToken     = errors.New("token has been revoked")
	ErrTokenReuse       = errors.New("refresh token already used; all sessions have been revoked")
	ErrInvalidAlgorithm = errors.New("invalid token signing algorithm")

	// Email verification errors
	ErrVerificationTokenInvalid = errors.New("invalid verification token")
	ErrVerificationTokenExpired = errors.New("verification token has expired")
	ErrVerificationTokenUsed    = errors.New("verification token has already been used")

	// Organization errors
	ErrOrganizationNotFound = errors.New("organization not found or access denied")
)

// ErrOrganizationRequired is returned when a user belongs to multiple
// organizations and no organization_id was provided in the login request.
// Organizations contains the available choices so the caller can present a
// selection UI without needing a separate round trip.
type ErrOrganizationRequired struct {
	Organizations []OrgSummary
}

func (e *ErrOrganizationRequired) Error() string {
	return fmt.Sprintf("organization_id required: user belongs to %d organizations", len(e.Organizations))
}
