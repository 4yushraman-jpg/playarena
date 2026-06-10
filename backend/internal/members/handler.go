package members

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/4yushraman-jpg/playarena/internal/auth"
	"github.com/4yushraman-jpg/playarena/internal/platform/response"
	"github.com/4yushraman-jpg/playarena/internal/platform/validator"
)

// Handler exposes the members service over HTTP.
type Handler struct {
	svc *Service
	log *slog.Logger
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// ── endpoints ─────────────────────────────────────────────────────────────────

// List handles GET /api/v1/organizations/{slug}/members.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	resp, err := h.svc.ListMembers(r.Context(), slug, principal.OrganizationID)
	if err != nil {
		h.log.WarnContext(r.Context(), "members.list.failed",
			slog.String("org_slug", slug),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeError(w, r, err)
		return
	}

	response.Write(w, http.StatusOK, resp)
}

// GetMember handles GET /api/v1/organizations/{slug}/members/{userID}.
func (h *Handler) GetMember(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	userID := chi.URLParam(r, "userID")

	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	member, err := h.svc.GetMember(r.Context(), slug, userID, principal.OrganizationID)
	if err != nil {
		h.log.WarnContext(r.Context(), "members.get.failed",
			slog.String("org_slug", slug),
			slog.String("user_id", userID),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeError(w, r, err)
		return
	}

	response.Write(w, http.StatusOK, member)
}

// GrantRole handles POST /api/v1/organizations/{slug}/members/{userID}/roles.
func (h *Handler) GrantRole(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	userID := chi.URLParam(r, "userID")

	var req GrantRequest
	if err := validator.DecodeJSON(r, &req); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	member, err := h.svc.GrantRole(r.Context(), slug, userID, req, principal.UserID, principal.OrganizationID)
	if err != nil {
		h.log.WarnContext(r.Context(), "members.grant_role.failed",
			slog.String("org_slug", slug),
			slog.String("user_id", userID),
			slog.String("role_slug", req.RoleSlug),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "members.grant_role.success",
		slog.String("org_slug", slug),
		slog.String("user_id", userID),
		slog.String("role_slug", req.RoleSlug),
		slog.String("actor_id", principal.UserID),
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	response.Write(w, http.StatusCreated, member)
}

// RevokeRole handles DELETE /api/v1/organizations/{slug}/members/{userID}/roles/{roleSlug}.
func (h *Handler) RevokeRole(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	userID := chi.URLParam(r, "userID")
	roleSlug := chi.URLParam(r, "roleSlug")

	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	if err := h.svc.RevokeRole(r.Context(), slug, userID, roleSlug, principal.UserID, principal.OrganizationID); err != nil {
		h.log.WarnContext(r.Context(), "members.revoke_role.failed",
			slog.String("org_slug", slug),
			slog.String("user_id", userID),
			slog.String("role_slug", roleSlug),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "members.revoke_role.success",
		slog.String("org_slug", slug),
		slog.String("user_id", userID),
		slog.String("role_slug", roleSlug),
		slog.String("actor_id", principal.UserID),
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	w.WriteHeader(http.StatusNoContent)
}

// ── error handling ────────────────────────────────────────────────────────────

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrOrganizationNotFound):
		response.Error(w, http.StatusNotFound, "organization not found")
	case errors.Is(err, ErrUserNotFound):
		response.Error(w, http.StatusNotFound, "user not found")
	case errors.Is(err, ErrRoleNotFound):
		response.Error(w, http.StatusNotFound, "role not found")
	case errors.Is(err, ErrGrantNotFound):
		response.Error(w, http.StatusNotFound, "role grant not found")
	case errors.Is(err, ErrForbidden):
		response.Error(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrLastOwner):
		response.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrInvalidExpiresAt):
		response.Error(w, http.StatusBadRequest, err.Error())
	default:
		h.log.ErrorContext(r.Context(), "members.unhandled_error", slog.Any("error", err))
		response.Error(w, http.StatusInternalServerError, "internal server error")
	}
}
