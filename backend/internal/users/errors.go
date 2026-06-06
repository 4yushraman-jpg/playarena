package users

import (
	"errors"
	"fmt"
)

var (
	ErrUserNotFound         = errors.New("user not found")
	ErrForbidden            = errors.New("access denied")
	ErrUsernameAlreadyTaken = errors.New("username is already taken")
	ErrUserAlreadyInactive  = errors.New("user account is already inactive")
	ErrLastPlatformAdmin    = errors.New("cannot deactivate the last platform administrator")
	ErrInvalidCredentials   = errors.New("current password is incorrect")
	ErrEmailNotUpdatable    = errors.New("email cannot be updated via this endpoint")
)

// ErrBadRequest is a field-level validation failure safe to surface to the caller.
// The Field is the JSON key; Message is a human-readable constraint description.
type ErrBadRequest struct {
	Field   string
	Message string
}

func (e *ErrBadRequest) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func badRequest(field, msg string) *ErrBadRequest {
	return &ErrBadRequest{Field: field, Message: msg}
}
