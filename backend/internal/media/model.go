package media

const (
	DefaultListLimit int32 = 50
	MaxListLimit     int32 = 200

	// MaxImageBytes is the hard upload size limit enforced before any parsing.
	MaxImageBytes int64 = 10 * 1024 * 1024 // 10 MB

	// formOverhead is added to MaxImageBytes when wrapping the request body to
	// account for multipart form framing (boundary, field headers, etc.).
	formOverhead int64 = 64 * 1024 // 64 KB
)

// ListParams carries validated query parameters for listing media.
type ListParams struct {
	EntityType *string
	EntityID   *string
	Limit      int32
	Offset     int32
}

// VariantsMeta is the JSON shape stored in metadata.variants for each
// attachment. Keys are stored in the DB row; URLs are derived at serve time.
type VariantsMeta struct {
	FullKey string `json:"full"` // storage key for the full-size WebP
	SMKey   string `json:"sm"`   // storage key for the 150px thumbnail
	MDKey   string `json:"md"`   // storage key for the 400px thumbnail
}
