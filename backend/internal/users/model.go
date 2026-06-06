package users

const (
	DefaultListLimit = int32(50)
	MaxListLimit     = int32(200)
)

// ListParams carries validated pagination inputs for GET /api/v1/users.
type ListParams struct {
	Limit  int32
	Offset int32
}
