package members

// ── request DTOs ──────────────────────────────────────────────────────────────

// GrantRequest is the payload for POST /organizations/{slug}/members/{userID}/roles.
type GrantRequest struct {
	RoleSlug  string  `json:"role_slug"  validate:"required,min=1,max=100"`
	ExpiresAt *string `json:"expires_at"` // optional RFC3339; nil = no expiry
}

// ── response DTOs ─────────────────────────────────────────────────────────────

// RoleGrant is one active role held by a member.
type RoleGrant struct {
	GrantID   string  `json:"grant_id"`
	RoleSlug  string  `json:"role_slug"`
	RoleName  string  `json:"role_name"`
	GrantedAt string  `json:"granted_at"`
	ExpiresAt *string `json:"expires_at,omitempty"`
	GrantedBy *string `json:"granted_by,omitempty"`
}

// MemberResponse represents a user and all their active roles in an org.
type MemberResponse struct {
	UserID     string      `json:"user_id"`
	Email      string      `json:"email"`
	Username   string      `json:"username"`
	FirstName  string      `json:"first_name"`
	LastName   string      `json:"last_name"`
	UserStatus string      `json:"user_status"`
	Roles      []RoleGrant `json:"roles"`
}

// ListResponse wraps the full member roster.
type ListResponse struct {
	Members []MemberResponse `json:"members"`
}
