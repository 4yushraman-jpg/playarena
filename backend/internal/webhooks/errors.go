package webhooks

import "errors"

var (
	ErrOrganizationNotFound = errors.New("organization not found")
	ErrWebhookNotFound      = errors.New("webhook endpoint not found")
	ErrForbidden            = errors.New("access denied: you do not have permission to manage this webhook")
	ErrInvalidURL           = errors.New("webhook URL must use HTTPS and be a publicly reachable host")
	ErrSSRFBlocked          = errors.New("webhook URL resolves to a private or reserved IP address")
	ErrURLRequired          = errors.New("url is required")
)
