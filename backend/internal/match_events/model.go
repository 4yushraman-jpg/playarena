package match_events

const (
	MaxListLimit     = int32(200)
	DefaultListLimit = int32(50)
)

// ListParams carries validated pagination and filter inputs for event listings.
type ListParams struct {
	Limit         int32
	Offset        int32
	EffectiveOnly bool // when true, events cancelled by score_correction are excluded
}
