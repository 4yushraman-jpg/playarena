package media

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/platform/response"
	"github.com/4yushraman-jpg/playarena/internal/platform/validator"
)

// Handler exposes the media service over HTTP.
type Handler struct {
	svc *Service
	log *slog.Logger
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// ── endpoints ─────────────────────────────────────────────────────────────────

// Upload handles POST /api/v1/organizations/{slug}/media.
// Accepts multipart/form-data with fields:
//   - file        (required) — the image file
//   - entity_type (required) — organization | team | player | tournament
//   - entity_id   (required) — UUID of the target entity
//   - alt_text    (optional) — accessibility description
//   - is_primary  (optional) — "true" to set as primary for the entity
//
// The handler enforces MaxBytesReader BEFORE any parsing so oversized requests
// are rejected at the HTTP layer, not by the image decoder.
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	// Bound the request body before any reading (10 MB image + 64 KB overhead).
	r.Body = http.MaxBytesReader(w, r.Body, MaxImageBytes+formOverhead)

	// Parse multipart: only 1 MB is held in memory; remainder is streamed to
	// a temp file by the stdlib. This prevents large uploads from filling RAM.
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		response.Error(w, http.StatusRequestEntityTooLarge, "file exceeds the maximum allowed size (10 MB)")
		return
	}
	defer r.MultipartForm.RemoveAll() //nolint:errcheck

	// Extract the file part. FormFile returns (multipart.File, *FileHeader, error).
	fileReader, fileHeader, err := r.FormFile("file")
	if err != nil {
		response.Error(w, http.StatusBadRequest, "missing required field: file")
		return
	}
	defer fileReader.Close()

	// Read the file into memory (already bounded by MaxBytesReader).
	fileData, err := io.ReadAll(fileReader)
	if err != nil {
		response.Error(w, http.StatusRequestEntityTooLarge, "file exceeds the maximum allowed size (10 MB)")
		return
	}

	// Extract form fields.
	entityType := r.FormValue("entity_type")
	entityID := r.FormValue("entity_id")
	altText := r.FormValue("alt_text")
	isPrimary := r.FormValue("is_primary") == "true"

	if entityType == "" {
		response.Error(w, http.StatusBadRequest, "missing required field: entity_type")
		return
	}
	if entityID == "" {
		response.Error(w, http.StatusBadRequest, "missing required field: entity_id")
		return
	}

	// Determine a safe original filename from the Content-Disposition header.
	originalName := fileHeader.Filename
	if originalName == "" {
		originalName = "upload"
	}

	att, err := h.svc.Upload(
		r.Context(),
		slug,
		entityType,
		entityID,
		altText,
		isPrimary,
		originalName,
		fileData,
		principal.UserID,
		principal.OrganizationID,
	)
	if err != nil {
		h.log.WarnContext(r.Context(), "media.upload.failed",
			slog.String("org_slug", slug),
			slog.String("error_kind", errKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "media.upload.success",
		slog.String("attachment_id", att.ID),
		slog.String("org_slug", slug),
		slog.String("entity_type", att.EntityType),
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	response.Write(w, http.StatusCreated, att)
}

// List handles GET /api/v1/organizations/{slug}/media.
// Supports ?entity_type=, ?entity_id=, ?limit=, ?offset= query parameters.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	params := ListParams{
		Limit:  DefaultListLimit,
		Offset: 0,
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 32); err == nil {
			params.Limit = int32(n)
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 32); err == nil {
			params.Offset = int32(n)
		}
	}
	if v := r.URL.Query().Get("entity_type"); v != "" {
		params.EntityType = &v
	}
	if v := r.URL.Query().Get("entity_id"); v != "" {
		params.EntityID = &v
	}

	list, err := h.svc.List(r.Context(), slug, params, principal.OrganizationID)
	if err != nil {
		if errors.Is(err, ErrOrganizationNotFound) {
			response.Error(w, http.StatusNotFound, "organization not found")
			return
		}
		h.log.ErrorContext(r.Context(), "media.list.failed",
			slog.String("org_slug", slug),
			slog.Any("error", err),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	response.Write(w, http.StatusOK, list)
}

// GetByID handles GET /api/v1/organizations/{slug}/media/{id}.
func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	id := chi.URLParam(r, "id")

	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	att, err := h.svc.GetByID(r.Context(), slug, id, principal.OrganizationID)
	if err != nil {
		if errors.Is(err, ErrOrganizationNotFound) {
			response.Error(w, http.StatusNotFound, "organization not found")
			return
		}
		if errors.Is(err, ErrNotFound) {
			response.Error(w, http.StatusNotFound, "media attachment not found")
			return
		}
		h.log.ErrorContext(r.Context(), "media.get.failed",
			slog.String("org_slug", slug),
			slog.String("attachment_id", id),
			slog.Any("error", err),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	response.Write(w, http.StatusOK, att)
}

// Update handles PATCH /api/v1/organizations/{slug}/media/{id}.
// Accepts JSON body with optional fields: alt_text, sort_order, is_primary.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	id := chi.URLParam(r, "id")

	var req UpdateRequest
	if err := validator.DecodeJSON(r, &req); err != nil {
		h.writeDecodeError(w, err)
		return
	}

	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	att, err := h.svc.Update(r.Context(), slug, id, req, principal.UserID, principal.OrganizationID)
	if err != nil {
		h.log.WarnContext(r.Context(), "media.update.failed",
			slog.String("org_slug", slug),
			slog.String("attachment_id", id),
			slog.String("error_kind", errKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "media.update.success",
		slog.String("attachment_id", att.ID),
		slog.String("org_slug", slug),
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	response.Write(w, http.StatusOK, att)
}

// Delete handles DELETE /api/v1/organizations/{slug}/media/{id}.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	id := chi.URLParam(r, "id")

	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	if err := h.svc.Delete(r.Context(), slug, id, principal.UserID, principal.OrganizationID); err != nil {
		h.log.WarnContext(r.Context(), "media.delete.failed",
			slog.String("org_slug", slug),
			slog.String("attachment_id", id),
			slog.String("error_kind", errKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "media.delete.success",
		slog.String("attachment_id", id),
		slog.String("org_slug", slug),
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	w.WriteHeader(http.StatusNoContent)
}

// ── error handling ─────────────────────────────────────────────────────────────

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrOrganizationNotFound):
		response.Error(w, http.StatusNotFound, "organization not found")
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusNotFound, "media attachment not found")
	case errors.Is(err, ErrForbidden):
		response.Error(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrEntityNotFound):
		response.Error(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrUnsupportedEntityType):
		response.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrUnsupportedMIME):
		response.Error(w, http.StatusUnsupportedMediaType, err.Error())
	case errors.Is(err, ErrFileTooLarge):
		response.Error(w, http.StatusRequestEntityTooLarge, err.Error())
	case errors.Is(err, ErrInvalidEntityID):
		response.Error(w, http.StatusBadRequest, err.Error())
	default:
		h.log.ErrorContext(r.Context(), "media.unexpected_error",
			slog.Any("error", err),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		response.Error(w, http.StatusInternalServerError, "internal server error")
	}
}

func (h *Handler) writeDecodeError(w http.ResponseWriter, err error) {
	var ve *validator.ValidationError
	if errors.As(err, &ve) {
		response.Write(w, http.StatusBadRequest, struct {
			Error  string            `json:"error"`
			Fields map[string]string `json:"fields"`
		}{
			Error:  "validation failed",
			Fields: ve.Fields,
		})
		return
	}
	response.Error(w, http.StatusBadRequest, err.Error())
}

// errKind returns a stable, loggable string for the error type.
func errKind(err error) string {
	switch {
	case errors.Is(err, ErrOrganizationNotFound):
		return "org_not_found"
	case errors.Is(err, ErrNotFound):
		return "not_found"
	case errors.Is(err, ErrForbidden):
		return "forbidden"
	case errors.Is(err, ErrEntityNotFound):
		return "entity_not_found"
	case errors.Is(err, ErrUnsupportedEntityType):
		return "unsupported_entity_type"
	case errors.Is(err, ErrUnsupportedMIME):
		return "unsupported_mime"
	case errors.Is(err, ErrFileTooLarge):
		return "file_too_large"
	case errors.Is(err, ErrInvalidEntityID):
		return "invalid_entity_id"
	default:
		return "internal_error"
	}
}
