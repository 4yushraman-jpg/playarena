package rankings

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/4yushraman-jpg/playarena/internal/platform/response"
)

// Handler exposes the rankings service over HTTP.
type Handler struct {
	svc *Service
	log *slog.Logger
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// ListPlayerRankings handles GET /api/v1/organizations/{slug}/rankings/players.
func (h *Handler) ListPlayerRankings(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	params := h.parseListParams(r)

	resp, err := h.svc.ListPlayerRankings(r.Context(), slug, params)
	if err != nil {
		if errors.Is(err, ErrOrganizationNotFound) {
			response.Error(w, http.StatusNotFound, "organization not found")
			return
		}
		h.log.ErrorContext(r.Context(), "rankings.list_players.failed",
			slog.String("org_slug", slug),
			slog.Any("error", err),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	response.Write(w, http.StatusOK, resp)
}

// ListTeamRankings handles GET /api/v1/organizations/{slug}/rankings/teams.
func (h *Handler) ListTeamRankings(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	params := h.parseListParams(r)

	resp, err := h.svc.ListTeamRankings(r.Context(), slug, params)
	if err != nil {
		if errors.Is(err, ErrOrganizationNotFound) {
			response.Error(w, http.StatusNotFound, "organization not found")
			return
		}
		h.log.ErrorContext(r.Context(), "rankings.list_teams.failed",
			slog.String("org_slug", slug),
			slog.Any("error", err),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	response.Write(w, http.StatusOK, resp)
}

func (h *Handler) parseListParams(r *http.Request) ListParams {
	params := ListParams{Limit: DefaultListLimit, Offset: 0}
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
	return params
}
