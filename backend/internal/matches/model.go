package matches

const (
	MaxListLimit     = int32(200)
	DefaultListLimit = int32(50)
)

// ListParams carries validated pagination and filter inputs for match listings.
type ListParams struct {
	Limit            int32
	Offset           int32
	TournamentFilter *string // optional UUID string; filters to a single tournament
	StatusFilter     *string
	Search           *string // searches venue and round_name (ILIKE)
}
