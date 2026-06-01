package teams

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

// Handler exposes the teams service over HTTP.
type Handler struct {
	svc *Service
	log *slog.Logger
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// ── team CRUD endpoints ───────────────────────────────────────────────────────

// Create handles POST /api/v1/organizations/{slug}/teams.
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

	team, err := h.svc.Create(r.Context(), slug, req, principal.UserID, principal.OrganizationID)
	if err != nil {
		h.log.WarnContext(r.Context(), "teams.create.failed",
			slog.String("org_slug", slug),
			slog.String("error_kind", errKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeTeamError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "teams.create.success",
		slog.String("team_id", team.ID),
		slog.String("slug", team.Slug),
		slog.String("org_slug", slug),
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	response.Write(w, http.StatusCreated, team)
}

// List handles GET /api/v1/organizations/{slug}/teams.
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
		h.log.ErrorContext(r.Context(), "teams.list.failed",
			slog.String("org_slug", slug),
			slog.Any("error", err),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	response.Write(w, http.StatusOK, list)
}

// GetByID handles GET /api/v1/organizations/{slug}/teams/{id}.
// Returns the team regardless of status. Disbanded teams are returned with
// "status": "disbanded" so that historical data remains accessible.
func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	id := chi.URLParam(r, "id")

	team, err := h.svc.GetByID(r.Context(), slug, id)
	if err != nil {
		if errors.Is(err, ErrOrganizationNotFound) {
			response.Error(w, http.StatusNotFound, "organization not found")
			return
		}
		if errors.Is(err, ErrTeamNotFound) {
			response.Error(w, http.StatusNotFound, "team not found")
			return
		}
		h.log.ErrorContext(r.Context(), "teams.get.failed",
			slog.String("org_slug", slug),
			slog.String("team_id", id),
			slog.Any("error", err),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	response.Write(w, http.StatusOK, team)
}

// Update handles PATCH /api/v1/organizations/{slug}/teams/{id}.
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

	team, err := h.svc.Update(r.Context(), slug, id, req, principal.UserID, principal.OrganizationID)
	if err != nil {
		h.log.WarnContext(r.Context(), "teams.update.failed",
			slog.String("org_slug", slug),
			slog.String("team_id", id),
			slog.String("error_kind", errKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeTeamError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "teams.update.success",
		slog.String("team_id", team.ID),
		slog.String("org_slug", slug),
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	response.Write(w, http.StatusOK, team)
}

// Delete handles DELETE /api/v1/organizations/{slug}/teams/{id}.
// Soft-deletes by setting team status to disbanded. No hard deletes.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	id := chi.URLParam(r, "id")

	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	if err := h.svc.Delete(r.Context(), slug, id, principal.UserID, principal.OrganizationID); err != nil {
		h.log.WarnContext(r.Context(), "teams.delete.failed",
			slog.String("org_slug", slug),
			slog.String("team_id", id),
			slog.String("error_kind", errKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeTeamError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "teams.delete.success",
		slog.String("team_id", id),
		slog.String("org_slug", slug),
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	w.WriteHeader(http.StatusNoContent)
}

// ── membership endpoints ──────────────────────────────────────────────────────

// AddMember handles POST /api/v1/organizations/{slug}/teams/{id}/members.
func (h *Handler) AddMember(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	id := chi.URLParam(r, "id")

	var req AddMemberRequest
	if err := validator.DecodeJSON(r, &req); err != nil {
		h.writeDecodeError(w, err)
		return
	}

	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	m, err := h.svc.AddMember(r.Context(), slug, id, req, principal.UserID, principal.OrganizationID)
	if err != nil {
		h.log.WarnContext(r.Context(), "teams.members.add.failed",
			slog.String("org_slug", slug),
			slog.String("team_id", id),
			slog.String("error_kind", errKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeTeamError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "teams.members.add.success",
		slog.String("membership_id", m.ID),
		slog.String("team_id", id),
		slog.String("player_id", m.PlayerID),
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	response.Write(w, http.StatusCreated, m)
}

// ListMembers handles GET /api/v1/organizations/{slug}/teams/{id}/members.
func (h *Handler) ListMembers(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	id := chi.URLParam(r, "id")

	list, err := h.svc.ListMembers(r.Context(), slug, id)
	if err != nil {
		if errors.Is(err, ErrOrganizationNotFound) {
			response.Error(w, http.StatusNotFound, "organization not found")
			return
		}
		if errors.Is(err, ErrTeamNotFound) {
			response.Error(w, http.StatusNotFound, "team not found")
			return
		}
		h.log.ErrorContext(r.Context(), "teams.members.list.failed",
			slog.String("org_slug", slug),
			slog.String("team_id", id),
			slog.Any("error", err),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	response.Write(w, http.StatusOK, list)
}

// RemoveMember handles DELETE /api/v1/organizations/{slug}/teams/{id}/members/{membershipId}.
func (h *Handler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	id := chi.URLParam(r, "id")
	membershipID := chi.URLParam(r, "membershipId")

	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	if err := h.svc.RemoveMember(r.Context(), slug, id, membershipID, principal.UserID, principal.OrganizationID); err != nil {
		h.log.WarnContext(r.Context(), "teams.members.remove.failed",
			slog.String("org_slug", slug),
			slog.String("team_id", id),
			slog.String("membership_id", membershipID),
			slog.String("error_kind", errKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeTeamError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "teams.members.remove.success",
		slog.String("membership_id", membershipID),
		slog.String("team_id", id),
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	w.WriteHeader(http.StatusNoContent)
}

// ── error handling ────────────────────────────────────────────────────────────

func (h *Handler) writeTeamError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrOrganizationNotFound):
		response.Error(w, http.StatusNotFound, "organization not found")
	case errors.Is(err, ErrTeamNotFound):
		response.Error(w, http.StatusNotFound, "team not found")
	case errors.Is(err, ErrPlayerNotFound):
		response.Error(w, http.StatusNotFound, "player not found")
	case errors.Is(err, ErrMembershipNotFound):
		response.Error(w, http.StatusNotFound, "membership not found")
	case errors.Is(err, ErrForbidden):
		response.Error(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrSlugAlreadyTaken), errors.Is(err, ErrSlugGenerationFailed):
		response.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrMembershipAlreadyActive):
		response.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrCrossOrgMembership):
		response.Error(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrInvalidStatus):
		response.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrInvalidColor):
		response.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrInvalidShortName):
		response.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrInvalidMembershipRole):
		response.Error(w, http.StatusBadRequest, err.Error())
	default:
		h.log.ErrorContext(r.Context(), "teams.unexpected_error",
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
	case errors.Is(err, ErrTeamNotFound):
		return "team_not_found"
	case errors.Is(err, ErrPlayerNotFound):
		return "player_not_found"
	case errors.Is(err, ErrMembershipNotFound):
		return "membership_not_found"
	case errors.Is(err, ErrForbidden):
		return "forbidden"
	case errors.Is(err, ErrSlugAlreadyTaken):
		return "slug_taken"
	case errors.Is(err, ErrSlugGenerationFailed):
		return "slug_generation_failed"
	case errors.Is(err, ErrMembershipAlreadyActive):
		return "membership_already_active"
	case errors.Is(err, ErrCrossOrgMembership):
		return "cross_org_membership"
	case errors.Is(err, ErrInvalidStatus):
		return "invalid_status"
	case errors.Is(err, ErrInvalidColor):
		return "invalid_color"
	case errors.Is(err, ErrInvalidShortName):
		return "invalid_short_name"
	case errors.Is(err, ErrInvalidMembershipRole):
		return "invalid_membership_role"
	default:
		return "internal_error"
	}
}
