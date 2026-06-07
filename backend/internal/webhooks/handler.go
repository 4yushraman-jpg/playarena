package webhooks

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/platform/response"
	"github.com/4yushraman-jpg/playarena/internal/platform/validator"
)

// Handler exposes the webhooks service over HTTP.
type Handler struct {
	svc *Service
	log *slog.Logger
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// Create handles POST /api/v1/organizations/{slug}/webhooks.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateRequest
	if err := validator.DecodeJSON(r, &req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	orgID, ok := h.resolveOrgID(w, r)
	if !ok {
		return
	}

	resp, err := h.svc.Create(r.Context(), orgID, principal.UserID, req)
	if err != nil {
		h.log.WarnContext(r.Context(), "webhooks.create.failed",
			slog.String("error", err.Error()),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeError(w, err)
		return
	}

	h.log.InfoContext(r.Context(), "webhooks.create.success",
		slog.String("webhook_id", resp.ID),
		slog.String("url", resp.URL),
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	response.Write(w, http.StatusCreated, resp)
}

// List handles GET /api/v1/organizations/{slug}/webhooks.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.resolveOrgID(w, r)
	if !ok {
		return
	}

	resp, err := h.svc.List(r.Context(), orgID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.Write(w, http.StatusOK, resp)
}

// GetByID handles GET /api/v1/organizations/{slug}/webhooks/{webhookID}.
func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.resolveOrgID(w, r)
	if !ok {
		return
	}
	webhookID := chi.URLParam(r, "webhookID")

	resp, err := h.svc.GetByID(r.Context(), orgID, webhookID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.Write(w, http.StatusOK, resp)
}

// UpdateActive handles PATCH /api/v1/organizations/{slug}/webhooks/{webhookID}/active.
func (h *Handler) UpdateActive(w http.ResponseWriter, r *http.Request) {
	var req UpdateActiveRequest
	if err := validator.DecodeJSON(r, &req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	orgID, ok := h.resolveOrgID(w, r)
	if !ok {
		return
	}
	webhookID := chi.URLParam(r, "webhookID")

	resp, err := h.svc.UpdateActive(r.Context(), orgID, webhookID, req.Active)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.Write(w, http.StatusOK, resp)
}

// Delete handles DELETE /api/v1/organizations/{slug}/webhooks/{webhookID}.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.resolveOrgID(w, r)
	if !ok {
		return
	}
	webhookID := chi.URLParam(r, "webhookID")

	if err := h.svc.Delete(r.Context(), orgID, webhookID); err != nil {
		h.writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── helpers ───────────────────────────────────────────────────────────────────

// resolveOrgID looks up the organization by slug from the URL parameter.
// Writes a 404 response and returns false when not found.
func (h *Handler) resolveOrgID(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	slug := chi.URLParam(r, "slug")
	org, err := h.svc.repo.GetOrgBySlug(r.Context(), slug)
	if err != nil {
		h.writeError(w, err)
		return pgtype.UUID{}, false
	}
	return org.ID, true
}

func (h *Handler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrOrganizationNotFound):
		response.Error(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrWebhookNotFound):
		response.Error(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrForbidden):
		response.Error(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrInvalidURL), errors.Is(err, ErrURLRequired):
		response.Error(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrSSRFBlocked):
		response.Error(w, http.StatusUnprocessableEntity, err.Error())
	default:
		response.Error(w, http.StatusInternalServerError, "internal server error")
	}
}
