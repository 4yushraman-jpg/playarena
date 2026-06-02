package notifications

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/platform/response"
	"github.com/4yushraman-jpg/playarena/internal/platform/validator"
)

// Handler exposes the notifications service over HTTP.
type Handler struct {
	svc *Service
	log *slog.Logger
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// ── notification endpoints ────────────────────────────────────────────────────

// List handles GET /api/v1/organizations/{slug}/notifications.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	limit, offset := parsePagination(r)
	resp, err := h.svc.List(r.Context(), slug, principal.UserID, ListParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	response.Write(w, http.StatusOK, resp)
}

// GetByID handles GET /api/v1/organizations/{slug}/notifications/{id}.
func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	id := chi.URLParam(r, "id")
	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	resp, err := h.svc.GetByID(r.Context(), slug, id, principal.UserID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	response.Write(w, http.StatusOK, resp)
}

// MarkRead handles PATCH /api/v1/organizations/{slug}/notifications/{id}/read.
func (h *Handler) MarkRead(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	id := chi.URLParam(r, "id")
	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	resp, err := h.svc.MarkRead(r.Context(), slug, id, principal.UserID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	response.Write(w, http.StatusOK, resp)
}

// MarkAllRead handles POST /api/v1/organizations/{slug}/notifications/read-all.
func (h *Handler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	if err := h.svc.MarkAllRead(r.Context(), slug, principal.UserID); err != nil {
		h.handleError(w, r, err)
		return
	}
	response.Write(w, http.StatusNoContent, nil)
}

// Delete handles DELETE /api/v1/organizations/{slug}/notifications/{id}.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	id := chi.URLParam(r, "id")
	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	if err := h.svc.Delete(r.Context(), slug, id, principal.UserID); err != nil {
		h.handleError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── preference endpoints ──────────────────────────────────────────────────────

// GetPreferences handles GET /api/v1/organizations/{slug}/notifications/preferences.
func (h *Handler) GetPreferences(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	resp, err := h.svc.GetPreferences(r.Context(), slug, principal.UserID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	response.Write(w, http.StatusOK, resp)
}

// UpdatePreference handles PUT /api/v1/organizations/{slug}/notifications/preferences/{event_type}.
func (h *Handler) UpdatePreference(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	eventType := chi.URLParam(r, "event_type")
	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	var req UpdatePreferenceRequest
	if err := validator.DecodeJSON(r, &req); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := h.svc.UpdatePreference(r.Context(), slug, eventType, principal.UserID, principal.UserID, req)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	response.Write(w, http.StatusOK, resp)
}

// ── error mapping ─────────────────────────────────────────────────────────────

func (h *Handler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrOrganizationNotFound):
		response.Error(w, http.StatusNotFound, "organization not found")
	case errors.Is(err, ErrNotificationNotFound):
		response.Error(w, http.StatusNotFound, "notification not found")
	case errors.Is(err, ErrPreferenceNotFound):
		response.Error(w, http.StatusNotFound, "preference not found")
	case errors.Is(err, ErrInvalidEventType):
		response.Error(w, http.StatusBadRequest, "invalid notification event type")
	case errors.Is(err, ErrInvalidChannel):
		response.Error(w, http.StatusBadRequest, "invalid notification channel")
	case errors.Is(err, ErrForbidden):
		response.Error(w, http.StatusForbidden, "forbidden")
	default:
		h.log.Error("notifications: unhandled error",
			slog.String("path", r.URL.Path),
			slog.Any("error", err),
		)
		response.Error(w, http.StatusInternalServerError, "internal server error")
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func parsePagination(r *http.Request) (limit, offset int) {
	limit = 50
	offset = 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	return limit, offset
}
