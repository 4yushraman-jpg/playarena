package notifications

import "errors"

var (
	ErrOrganizationNotFound = errors.New("notifications: organization not found")
	ErrNotificationNotFound = errors.New("notifications: notification not found")
	ErrPreferenceNotFound   = errors.New("notifications: preference not found")
	ErrInvalidEventType     = errors.New("notifications: invalid notification event type")
	ErrInvalidChannel       = errors.New("notifications: invalid notification channel")
	ErrForbidden            = errors.New("notifications: forbidden")
)
