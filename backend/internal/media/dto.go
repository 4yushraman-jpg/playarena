package media

// UploadRequest is populated from multipart/form-data fields (not JSON).
// The actual file is extracted from the "file" part by the handler.
type UploadRequest struct {
	EntityType string `form:"entity_type"` // required
	EntityID   string `form:"entity_id"`   // required, UUID
	AltText    string `form:"alt_text"`    // optional
	IsPrimary  bool   `form:"is_primary"`  // optional, default false
}

// UpdateRequest carries the mutable metadata fields for PATCH.
// All fields are optional — only non-nil values are applied.
type UpdateRequest struct {
	AltText   *string `json:"alt_text"`
	SortOrder *int16  `json:"sort_order"`
	IsPrimary *bool   `json:"is_primary"`
}

// Response is the canonical API representation of a media attachment.
// storage_key is intentionally omitted — it is an internal implementation
// detail. Clients receive file_url which is CDN-ready.
type Response struct {
	ID         string  `json:"id"`
	EntityType string  `json:"entity_type"`
	EntityID   string  `json:"entity_id"`
	MediaType  string  `json:"media_type"`
	FileName   string  `json:"file_name"`
	FileSize   *int64  `json:"file_size"`
	MimeType   *string `json:"mime_type"`
	Width      *int16  `json:"width"`
	Height     *int16  `json:"height"`
	AltText    *string `json:"alt_text"`
	IsPrimary  bool    `json:"is_primary"`
	SortOrder  int16   `json:"sort_order"`
	FileURL    string  `json:"file_url"`
	UploadedBy *string `json:"uploaded_by,omitempty"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
}

// ListResponse wraps a paginated slice of attachments with metadata.
type ListResponse struct {
	Attachments []Response `json:"attachments"`
	Total       int64      `json:"total"`
	Limit       int32      `json:"limit"`
	Offset      int32      `json:"offset"`
}
