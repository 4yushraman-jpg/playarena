package media

import "errors"

var (
	// ErrNotFound is returned when a media attachment does not exist in the
	// actor's organization (or at all).
	ErrNotFound = errors.New("media attachment not found")

	// ErrForbidden is returned when the actor's org context does not match the
	// target entity's org (BOLA protection).
	ErrForbidden = errors.New("access to this resource is forbidden")

	// ErrUnsupportedEntityType is returned when the entity_type in the upload
	// request is not supported in Phase 11 (e.g. "match", "user").
	ErrUnsupportedEntityType = errors.New("entity type not supported for media upload (supported: organization, team, player, tournament)")

	// ErrEntityNotFound is returned when the target entity (player, team, etc.)
	// does not exist in the actor's organization.
	ErrEntityNotFound = errors.New("target entity not found in your organization")

	// ErrUnsupportedMIME is returned when the detected MIME type is not in the
	// allowed list.
	ErrUnsupportedMIME = errors.New("unsupported file type (allowed: image/jpeg, image/png, image/webp, image/gif)")

	// ErrFileTooLarge is returned when the uploaded file exceeds the configured
	// size limit.
	ErrFileTooLarge = errors.New("file exceeds the maximum allowed size (10 MB)")

	// ErrInvalidEntityID is returned when entity_id is not a valid UUID.
	ErrInvalidEntityID = errors.New("entity_id must be a valid UUID")

	// ErrDuplicateContent is a sentinel that should never be returned to the
	// caller — the service returns the existing attachment instead. Kept here
	// for internal use.
	ErrDuplicateContent = errors.New("duplicate content: attachment already exists")

	// ErrOrganizationNotFound is returned when the org slug in the URL does not
	// resolve to a known organization.
	ErrOrganizationNotFound = errors.New("organization not found")
)
