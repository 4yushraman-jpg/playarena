package players

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

// Handler exposes the players service over HTTP.
type Handler struct {
	svc *Service
	log *slog.Logger
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// ── endpoints ─────────────────────────────────────────────────────────────────

// Create handles POST /api/v1/organizations/{slug}/players.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

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

	player, err := h.svc.Create(r.Context(), slug, req, principal.UserID, principal.OrganizationID)
	if err != nil {
		h.log.WarnContext(r.Context(), "players.create.failed",
			slog.String("org_slug", slug),
			slog.String("error_kind", errKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writePlayerError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "players.create.success",
		slog.String("player_id", player.ID),
		slog.String("org_slug", slug),
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	response.Write(w, http.StatusCreated, player)
}

// List handles GET /api/v1/organizations/{slug}/players.
// Supports ?limit=, ?offset=, ?status=, ?search= query parameters.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

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
	if v := r.URL.Query().Get("status"); v != "" {
		params.StatusFilter = &v
	}
	if v := r.URL.Query().Get("search"); v != "" {
		params.Search = &v
	}

	list, err := h.svc.List(r.Context(), slug, params)
	if err != nil {
		if errors.Is(err, ErrOrganizationNotFound) {
			response.Error(w, http.StatusNotFound, "organization not found")
			return
		}
		h.log.ErrorContext(r.Context(), "players.list.failed",
			slog.String("org_slug", slug),
			slog.Any("error", err),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	response.Write(w, http.StatusOK, list)
}

// GetByID handles GET /api/v1/organizations/{slug}/players/{id}.
// Returns the player regardless of status. Inactive (soft-deleted) players are
// returned with "status": "inactive" so that historical records remain
// accessible. Clients should check the status field when rendering UI.
func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	id := chi.URLParam(r, "id")

	player, err := h.svc.GetByID(r.Context(), slug, id)
	if err != nil {
		if errors.Is(err, ErrOrganizationNotFound) {
			response.Error(w, http.StatusNotFound, "organization not found")
			return
		}
		if errors.Is(err, ErrPlayerNotFound) {
			response.Error(w, http.StatusNotFound, "player not found")
			return
		}
		h.log.ErrorContext(r.Context(), "players.get.failed",
			slog.String("org_slug", slug),
			slog.String("player_id", id),
			slog.Any("error", err),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	response.Write(w, http.StatusOK, player)
}

// Update handles PATCH /api/v1/organizations/{slug}/players/{id}.
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

	player, err := h.svc.Update(r.Context(), slug, id, req, principal.UserID, principal.OrganizationID)
	if err != nil {
		h.log.WarnContext(r.Context(), "players.update.failed",
			slog.String("org_slug", slug),
			slog.String("player_id", id),
			slog.String("error_kind", errKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writePlayerError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "players.update.success",
		slog.String("player_id", player.ID),
		slog.String("org_slug", slug),
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	response.Write(w, http.StatusOK, player)
}

// Delete handles DELETE /api/v1/organizations/{slug}/players/{id}.
// Soft-deletes by setting player status to inactive. No hard deletes.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	id := chi.URLParam(r, "id")

	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	if err := h.svc.Delete(r.Context(), slug, id, principal.UserID, principal.OrganizationID); err != nil {
		h.log.WarnContext(r.Context(), "players.delete.failed",
			slog.String("org_slug", slug),
			slog.String("player_id", id),
			slog.String("error_kind", errKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writePlayerError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "players.delete.success",
		slog.String("player_id", id),
		slog.String("org_slug", slug),
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	w.WriteHeader(http.StatusNoContent)
}

// ── error handling ────────────────────────────────────────────────────────────

func (h *Handler) writePlayerError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrOrganizationNotFound):
		response.Error(w, http.StatusNotFound, "organization not found")
	case errors.Is(err, ErrPlayerNotFound):
		response.Error(w, http.StatusNotFound, "player not found")
	case errors.Is(err, ErrForbidden):
		response.Error(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrInvalidStatus):
		response.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrInvalidDominantHand):
		response.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrInvalidNationality):
		response.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrInvalidDateOfBirth):
		response.Error(w, http.StatusBadRequest, err.Error())
	default:
		h.log.ErrorContext(r.Context(), "players.unexpected_error",
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

// errKind returns a stable loggable string identifying the error type.
func errKind(err error) string {
	switch {
	case errors.Is(err, ErrOrganizationNotFound):
		return "org_not_found"
	case errors.Is(err, ErrPlayerNotFound):
		return "player_not_found"
	case errors.Is(err, ErrForbidden):
		return "forbidden"
	case errors.Is(err, ErrInvalidStatus):
		return "invalid_status"
	case errors.Is(err, ErrInvalidDominantHand):
		return "invalid_dominant_hand"
	case errors.Is(err, ErrInvalidNationality):
		return "invalid_nationality"
	case errors.Is(err, ErrInvalidDateOfBirth):
		return "invalid_date_of_birth"
	default:
		return "internal_error"
	}
}
