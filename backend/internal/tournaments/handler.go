package tournaments

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

// Handler exposes the tournaments service over HTTP.
type Handler struct {
	svc *Service
	log *slog.Logger
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// ── endpoints ─────────────────────────────────────────────────────────────────

// Create handles POST /api/v1/organizations/{slug}/tournaments.
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

	t, err := h.svc.Create(r.Context(), slug, req, principal.UserID, principal.OrganizationID)
	if err != nil {
		h.log.WarnContext(r.Context(), "tournaments.create.failed",
			slog.String("org_slug", slug),
			slog.String("error_kind", errKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeTournamentError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "tournaments.create.success",
		slog.String("tournament_id", t.ID),
		slog.String("slug", t.Slug),
		slog.String("org_slug", slug),
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	response.Write(w, http.StatusCreated, t)
}

// List handles GET /api/v1/organizations/{slug}/tournaments.
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
		h.log.ErrorContext(r.Context(), "tournaments.list.failed",
			slog.String("org_slug", slug),
			slog.Any("error", err),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	response.Write(w, http.StatusOK, list)
}

// GetByID handles GET /api/v1/organizations/{slug}/tournaments/{id}.
// Returns the tournament regardless of status. Cancelled tournaments are
// returned with "status": "cancelled" so that historical data remains accessible.
func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	id := chi.URLParam(r, "id")

	t, err := h.svc.GetByID(r.Context(), slug, id)
	if err != nil {
		if errors.Is(err, ErrOrganizationNotFound) {
			response.Error(w, http.StatusNotFound, "organization not found")
			return
		}
		if errors.Is(err, ErrTournamentNotFound) {
			response.Error(w, http.StatusNotFound, "tournament not found")
			return
		}
		h.log.ErrorContext(r.Context(), "tournaments.get.failed",
			slog.String("org_slug", slug),
			slog.String("tournament_id", id),
			slog.Any("error", err),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	response.Write(w, http.StatusOK, t)
}

// Update handles PATCH /api/v1/organizations/{slug}/tournaments/{id}.
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

	t, err := h.svc.Update(r.Context(), slug, id, req, principal.UserID, principal.OrganizationID)
	if err != nil {
		h.log.WarnContext(r.Context(), "tournaments.update.failed",
			slog.String("org_slug", slug),
			slog.String("tournament_id", id),
			slog.String("error_kind", errKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeTournamentError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "tournaments.update.success",
		slog.String("tournament_id", t.ID),
		slog.String("org_slug", slug),
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	response.Write(w, http.StatusOK, t)
}

// GetStandings handles GET /api/v1/organizations/{slug}/tournaments/{id}/standings.
// Derives current standings from snapshotted match scores.  No permission
// beyond RequireAuth is required — standings are readable by any authenticated user.
func (h *Handler) GetStandings(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	id := chi.URLParam(r, "id")

	resp, err := h.svc.GetStandings(r.Context(), slug, id)
	if err != nil {
		if errors.Is(err, ErrOrganizationNotFound) {
			response.Error(w, http.StatusNotFound, "organization not found")
			return
		}
		if errors.Is(err, ErrTournamentNotFound) {
			response.Error(w, http.StatusNotFound, "tournament not found")
			return
		}
		h.log.ErrorContext(r.Context(), "tournaments.standings.failed",
			slog.String("org_slug", slug),
			slog.String("tournament_id", id),
			slog.Any("error", err),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	response.Write(w, http.StatusOK, resp)
}

// Delete handles DELETE /api/v1/organizations/{slug}/tournaments/{id}.
// Soft-cancels by setting status to cancelled. No hard deletes.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	id := chi.URLParam(r, "id")

	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	if err := h.svc.Delete(r.Context(), slug, id, principal.UserID, principal.OrganizationID); err != nil {
		h.log.WarnContext(r.Context(), "tournaments.delete.failed",
			slog.String("org_slug", slug),
			slog.String("tournament_id", id),
			slog.String("error_kind", errKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeTournamentError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "tournaments.delete.success",
		slog.String("tournament_id", id),
		slog.String("org_slug", slug),
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	w.WriteHeader(http.StatusNoContent)
}

// ── error handling ────────────────────────────────────────────────────────────

func (h *Handler) writeTournamentError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrOrganizationNotFound):
		response.Error(w, http.StatusNotFound, "organization not found")
	case errors.Is(err, ErrTournamentNotFound):
		response.Error(w, http.StatusNotFound, "tournament not found")
	case errors.Is(err, ErrForbidden):
		response.Error(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrSlugAlreadyTaken), errors.Is(err, ErrSlugGenerationFailed):
		response.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrInvalidStatusTransition):
		response.Error(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrInvalidDateRange):
		response.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrInvalidFormat):
		response.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrInvalidParticipantType):
		response.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrInvalidStatus):
		response.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrInvalidCurrency):
		response.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrInvalidCountry):
		response.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrInvalidPrizePool):
		response.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrInvalidTimestamp):
		response.Error(w, http.StatusBadRequest, err.Error())
	default:
		h.log.ErrorContext(r.Context(), "tournaments.unexpected_error",
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

func errKind(err error) string {
	switch {
	case errors.Is(err, ErrOrganizationNotFound):
		return "org_not_found"
	case errors.Is(err, ErrTournamentNotFound):
		return "tournament_not_found"
	case errors.Is(err, ErrForbidden):
		return "forbidden"
	case errors.Is(err, ErrSlugAlreadyTaken):
		return "slug_taken"
	case errors.Is(err, ErrSlugGenerationFailed):
		return "slug_generation_failed"
	case errors.Is(err, ErrInvalidStatusTransition):
		return "invalid_status_transition"
	case errors.Is(err, ErrInvalidDateRange):
		return "invalid_date_range"
	case errors.Is(err, ErrInvalidFormat):
		return "invalid_format"
	case errors.Is(err, ErrInvalidParticipantType):
		return "invalid_participant_type"
	case errors.Is(err, ErrInvalidStatus):
		return "invalid_status"
	case errors.Is(err, ErrInvalidCurrency):
		return "invalid_currency"
	case errors.Is(err, ErrInvalidCountry):
		return "invalid_country"
	case errors.Is(err, ErrInvalidPrizePool):
		return "invalid_prize_pool"
	case errors.Is(err, ErrInvalidTimestamp):
		return "invalid_timestamp"
	default:
		return "internal_error"
	}
}
