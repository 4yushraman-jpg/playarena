package users

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

// Handler exposes the users service over HTTP.
type Handler struct {
	svc *Service
	log *slog.Logger
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// ── endpoints ─────────────────────────────────────────────────────────────────

// GetUser handles GET /api/v1/users/{id}.
// Allowed: self or platform admin.
func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}
	targetID := chi.URLParam(r, "id")

	user, err := h.svc.GetByID(r.Context(), principal.UserID, principal.IsPlatformUser(), targetID)
	if err != nil {
		h.writeUserError(w, r, err)
		return
	}
	response.Write(w, http.StatusOK, user)
}

// UpdateProfile handles PATCH /api/v1/users/{id}.
// Allowed: self or platform admin.
func (h *Handler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}
	targetID := chi.URLParam(r, "id")

	var req UpdateProfileRequest
	if err := validator.DecodeJSON(r, &req); err != nil {
		h.writeDecodeError(w, err)
		return
	}

	user, err := h.svc.UpdateProfile(r.Context(), principal.UserID, principal.IsPlatformUser(), targetID, req)
	if err != nil {
		h.log.WarnContext(r.Context(), "users.update_profile.failed",
			slog.String("target_id", targetID),
			slog.String("error_kind", errKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeUserError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "users.update_profile.success",
		slog.String("target_id", targetID),
		slog.String("actor_id", principal.UserID),
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	response.Write(w, http.StatusOK, user)
}

// ChangePassword handles POST /api/v1/users/{id}/change-password.
// Self-only: the actor must be the target user.
func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}
	targetID := chi.URLParam(r, "id")

	var req ChangePasswordRequest
	if err := validator.DecodeJSON(r, &req); err != nil {
		h.writeDecodeError(w, err)
		return
	}

	if err := h.svc.ChangePassword(r.Context(), principal.UserID, targetID, req); err != nil {
		h.log.WarnContext(r.Context(), "users.change_password.failed",
			slog.String("target_id", targetID),
			slog.String("error_kind", errKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeUserError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "users.change_password.success",
		slog.String("user_id", principal.UserID),
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	response.Write(w, http.StatusOK, struct {
		Message string `json:"message"`
	}{"password changed successfully; all sessions have been revoked"})
}

// ListUsers handles GET /api/v1/users.
// Platform admin only.
func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
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

	list, err := h.svc.ListUsers(r.Context(), principal.IsPlatformUser(), params)
	if err != nil {
		h.writeUserError(w, r, err)
		return
	}
	response.Write(w, http.StatusOK, list)
}

// DeactivateUser handles POST /api/v1/users/{id}/deactivate.
// Platform admin only.
func (h *Handler) DeactivateUser(w http.ResponseWriter, r *http.Request) {
	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}
	targetID := chi.URLParam(r, "id")

	if err := h.svc.DeactivateUser(r.Context(), principal.UserID, principal.IsPlatformUser(), targetID); err != nil {
		h.log.WarnContext(r.Context(), "users.deactivate.failed",
			slog.String("target_id", targetID),
			slog.String("error_kind", errKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeUserError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "users.deactivate.success",
		slog.String("target_id", targetID),
		slog.String("actor_id", principal.UserID),
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	w.WriteHeader(http.StatusNoContent)
}

// ── error handling ────────────────────────────────────────────────────────────

func (h *Handler) writeUserError(w http.ResponseWriter, r *http.Request, err error) {
	var badReq *ErrBadRequest
	switch {
	case errors.Is(err, ErrUserNotFound):
		response.Error(w, http.StatusNotFound, "user not found")
	case errors.Is(err, ErrForbidden):
		response.Error(w, http.StatusForbidden, "access denied")
	case errors.Is(err, ErrEmailNotUpdatable):
		response.Error(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrUsernameAlreadyTaken):
		response.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrLastPlatformAdmin):
		response.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrUserAlreadyInactive):
		response.Error(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrInvalidCredentials):
		response.Error(w, http.StatusUnauthorized, "current password is incorrect")
	case errors.As(err, &badReq):
		response.Write(w, http.StatusUnprocessableEntity, struct {
			Error  string `json:"error"`
			Field  string `json:"field"`
			Detail string `json:"detail"`
		}{"validation failed", badReq.Field, badReq.Message})
	default:
		h.log.ErrorContext(r.Context(), "users.unexpected_error",
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
		}{"validation failed", ve.Fields})
		return
	}
	if errors.Is(err, validator.ErrBodyTooLarge) {
		response.Error(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	response.Error(w, http.StatusBadRequest, err.Error())
}

func errKind(err error) string {
	var badReq *ErrBadRequest
	switch {
	case errors.Is(err, ErrUserNotFound):
		return "not_found"
	case errors.Is(err, ErrForbidden):
		return "forbidden"
	case errors.Is(err, ErrEmailNotUpdatable):
		return "email_not_updatable"
	case errors.Is(err, ErrUsernameAlreadyTaken):
		return "username_conflict"
	case errors.Is(err, ErrLastPlatformAdmin):
		return "last_platform_admin"
	case errors.Is(err, ErrUserAlreadyInactive):
		return "already_inactive"
	case errors.Is(err, ErrInvalidCredentials):
		return "invalid_credentials"
	case errors.As(err, &badReq):
		return "validation_error"
	default:
		return "internal"
	}
}
