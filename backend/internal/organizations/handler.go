package organizations

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/platform/response"
	"github.com/4yushraman-jpg/playarena/internal/platform/validator"
)

// Handler exposes the organizations service over HTTP.
type Handler struct {
	svc *Service
	log *slog.Logger
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// ── endpoints ─────────────────────────────────────────────────────────────────

// Create handles POST /api/v1/organizations.
// Protected by RequireAuth + RequirePermission("organization.create").
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateRequest
	if err := validator.DecodeJSON(r, &req); err != nil {
		h.writeDecodeError(w, err)
		return
	}

	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	org, err := h.svc.Create(r.Context(), req, principal.UserID)
	if err != nil {
		h.log.WarnContext(r.Context(), "organizations.create.failed",
			slog.String("error_kind", errKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeOrgError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "organizations.create.success",
		slog.String("org_id", org.ID),
		slog.String("slug", org.Slug),
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	response.Write(w, http.StatusCreated, org)
}

// List handles GET /api/v1/organizations.
// Returns all organizations ordered newest-first.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
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

	list, err := h.svc.List(r.Context(), params)
	if err != nil {
		h.log.ErrorContext(r.Context(), "organizations.list.failed",
			slog.Any("error", err),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	response.Write(w, http.StatusOK, list)
}

// GetBySlug handles GET /api/v1/organizations/{slug}.
func (h *Handler) GetBySlug(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	org, err := h.svc.GetBySlug(r.Context(), slug)
	if err != nil {
		if errors.Is(err, ErrOrganizationNotFound) {
			response.Error(w, http.StatusNotFound, "organization not found")
			return
		}
		h.log.ErrorContext(r.Context(), "organizations.get.failed",
			slog.Any("error", err),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	response.Write(w, http.StatusOK, org)
}

// Update handles PATCH /api/v1/organizations/{slug}.
// Protected by RequireAuth + RequirePermission("organization.update").
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

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

	org, err := h.svc.Update(r.Context(), slug, req, principal.UserID, principal.OrganizationID)
	if err != nil {
		h.log.WarnContext(r.Context(), "organizations.update.failed",
			slog.String("slug", slug),
			slog.String("error_kind", errKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeOrgError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "organizations.update.success",
		slog.String("org_id", org.ID),
		slog.String("slug", org.Slug),
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	response.Write(w, http.StatusOK, org)
}

// Delete handles DELETE /api/v1/organizations/{slug}.
// Protected by RequireAuth + RequirePermission("organization.delete").
// Hard-deletes the organization and all child records (via CASCADE).
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	if err := h.svc.Delete(r.Context(), slug, principal.UserID, principal.OrganizationID); err != nil {
		h.log.WarnContext(r.Context(), "organizations.delete.failed",
			slog.String("slug", slug),
			slog.String("error_kind", errKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeOrgError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "organizations.delete.success",
		slog.String("slug", slug),
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	w.WriteHeader(http.StatusNoContent)
}

// ── error handling ────────────────────────────────────────────────────────────

func (h *Handler) writeOrgError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrOrganizationNotFound):
		response.Error(w, http.StatusNotFound, "organization not found")
	case errors.Is(err, ErrForbidden):
		response.Error(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrSlugAlreadyTaken), errors.Is(err, ErrSlugGenerationFailed):
		response.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrInvalidOrgType):
		response.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrInvalidCountryCode):
		response.Error(w, http.StatusBadRequest, err.Error())
	default:
		h.log.ErrorContext(r.Context(), "organizations.unexpected_error",
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

// errKind returns a stable, loggable string identifying the error type.
func errKind(err error) string {
	switch {
	case errors.Is(err, ErrOrganizationNotFound):
		return "not_found"
	case errors.Is(err, ErrForbidden):
		return "forbidden"
	case errors.Is(err, ErrSlugAlreadyTaken):
		return "slug_taken"
	case errors.Is(err, ErrSlugGenerationFailed):
		return "slug_generation_failed"
	case errors.Is(err, ErrInvalidOrgType):
		return "invalid_org_type"
	case errors.Is(err, ErrInvalidCountryCode):
		return "invalid_country_code"
	default:
		return "internal_error"
	}
}
