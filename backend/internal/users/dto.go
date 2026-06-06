package users

// UpdateProfileRequest is the payload for PATCH /api/v1/users/{id}.
//
// Email is included so the decoder can detect it and return an explicit error —
// silently ignoring it would leave callers thinking their email was changed.
// All other fields are optional pointers: nil means "leave unchanged".
// An empty string (*field == "") for Phone, DateOfBirth, or Gender clears the field.
type UpdateProfileRequest struct {
	Email       *string `json:"email"`
	FirstName   *string `json:"first_name"`
	LastName    *string `json:"last_name"`
	Username    *string `json:"username"`
	Phone       *string `json:"phone"`
	DateOfBirth *string `json:"date_of_birth"`
	Gender      *string `json:"gender"`
}

// ChangePasswordRequest is the payload for POST /api/v1/users/{id}/change-password.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" validate:"required"`
	NewPassword     string `json:"new_password"     validate:"required,min=8,max=72"`
}

// UserResponse is the unified user profile returned by all endpoints.
// password_hash and internal fields are intentionally absent.
type UserResponse struct {
	ID          string  `json:"id"`
	Email       string  `json:"email"`
	Username    string  `json:"username"`
	FullName    string  `json:"full_name"`
	Phone       *string `json:"phone,omitempty"`
	DateOfBirth *string `json:"date_of_birth,omitempty"`
	Gender      *string `json:"gender,omitempty"`
	Status      string  `json:"status"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

// ListResponse wraps a page of users with pagination metadata.
type ListResponse struct {
	Users  []UserResponse `json:"users"`
	Total  int64          `json:"total"`
	Limit  int32          `json:"limit"`
	Offset int32          `json:"offset"`
}
