package tournament_registrations

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

// Handler exposes the tournament registrations service over HTTP.
type Handler struct {
	svc *Service
	log *slog.Logger
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// ── endpoints ─────────────────────────────────────────────────────────────────

// Register handles POST /api/v1/organizations/{slug}/tournaments/{tournamentId}/registrations.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	tournamentID := chi.URLParam(r, "tournamentId")

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

	reg, err := h.svc.Register(r.Context(), slug, tournamentID, req, principal.UserID, principal.OrganizationID)
	if err != nil {
		h.log.WarnContext(r.Context(), "registrations.create.failed",
			slog.String("org_slug", slug),
			slog.String("tournament_id", tournamentID),
			slog.String("error_kind", errKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeRegError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "registrations.create.success",
		slog.String("registration_id", reg.ID),
		slog.String("tournament_id", tournamentID),
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	response.Write(w, http.StatusCreated, reg)
}

// List handles GET /api/v1/organizations/{slug}/tournaments/{tournamentId}/registrations.
// Supports ?limit=, ?offset=, ?status= query parameters.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	tournamentID := chi.URLParam(r, "tournamentId")

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

	list, err := h.svc.List(r.Context(), slug, tournamentID, params)
	if err != nil {
		if errors.Is(err, ErrOrganizationNotFound) {
			response.Error(w, http.StatusNotFound, "organization not found")
			return
		}
		if errors.Is(err, ErrTournamentNotFound) {
			response.Error(w, http.StatusNotFound, "tournament not found")
			return
		}
		h.log.ErrorContext(r.Context(), "registrations.list.failed",
			slog.String("org_slug", slug),
			slog.String("tournament_id", tournamentID),
			slog.Any("error", err),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	response.Write(w, http.StatusOK, list)
}

// GetByID handles GET /api/v1/.../registrations/{registrationId}.
func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	tournamentID := chi.URLParam(r, "tournamentId")
	registrationID := chi.URLParam(r, "registrationId")

	reg, err := h.svc.GetByID(r.Context(), slug, tournamentID, registrationID)
	if err != nil {
		if errors.Is(err, ErrOrganizationNotFound) {
			response.Error(w, http.StatusNotFound, "organization not found")
			return
		}
		if errors.Is(err, ErrTournamentNotFound) {
			response.Error(w, http.StatusNotFound, "tournament not found")
			return
		}
		if errors.Is(err, ErrRegistrationNotFound) {
			response.Error(w, http.StatusNotFound, "registration not found")
			return
		}
		h.log.ErrorContext(r.Context(), "registrations.get.failed",
			slog.Any("error", err),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	response.Write(w, http.StatusOK, reg)
}

// Update handles PATCH /api/v1/.../registrations/{registrationId}.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	tournamentID := chi.URLParam(r, "tournamentId")
	registrationID := chi.URLParam(r, "registrationId")

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

	reg, err := h.svc.Update(r.Context(), slug, tournamentID, registrationID, req, principal.UserID, principal.OrganizationID)
	if err != nil {
		h.log.WarnContext(r.Context(), "registrations.update.failed",
			slog.String("registration_id", registrationID),
			slog.String("error_kind", errKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeRegError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "registrations.update.success",
		slog.String("registration_id", reg.ID),
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	response.Write(w, http.StatusOK, reg)
}

// Delete handles DELETE /api/v1/.../registrations/{registrationId}.
// Soft-withdraws the registration (status → withdrawn). No hard deletes.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	tournamentID := chi.URLParam(r, "tournamentId")
	registrationID := chi.URLParam(r, "registrationId")

	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	if err := h.svc.Withdraw(r.Context(), slug, tournamentID, registrationID, principal.UserID, principal.OrganizationID); err != nil {
		h.log.WarnContext(r.Context(), "registrations.delete.failed",
			slog.String("registration_id", registrationID),
			slog.String("error_kind", errKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeRegError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "registrations.delete.success",
		slog.String("registration_id", registrationID),
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	w.WriteHeader(http.StatusNoContent)
}

// ── error handling ────────────────────────────────────────────────────────────

func (h *Handler) writeRegError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrOrganizationNotFound):
		response.Error(w, http.StatusNotFound, "organization not found")
	case errors.Is(err, ErrTournamentNotFound):
		response.Error(w, http.StatusNotFound, "tournament not found")
	case errors.Is(err, ErrTeamNotFound):
		response.Error(w, http.StatusNotFound, "team not found")
	case errors.Is(err, ErrRegistrationNotFound):
		response.Error(w, http.StatusNotFound, "registration not found")
	case errors.Is(err, ErrForbidden):
		response.Error(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrAlreadyRegistered):
		response.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrTournamentFull):
		response.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrCrossOrgRegistration):
		response.Error(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrRegistrationClosed):
		response.Error(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrWindowNotOpen):
		response.Error(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrWindowClosed):
		response.Error(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrTeamNotActive):
		response.Error(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrEmptyTeam):
		response.Error(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrInvalidStatusTransition):
		response.Error(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrInvalidStatus):
		response.Error(w, http.StatusBadRequest, err.Error())
	default:
		h.log.ErrorContext(r.Context(), "registrations.unexpected_error",
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
	case errors.Is(err, ErrTeamNotFound):
		return "team_not_found"
	case errors.Is(err, ErrRegistrationNotFound):
		return "registration_not_found"
	case errors.Is(err, ErrForbidden):
		return "forbidden"
	case errors.Is(err, ErrAlreadyRegistered):
		return "already_registered"
	case errors.Is(err, ErrTournamentFull):
		return "tournament_full"
	case errors.Is(err, ErrCrossOrgRegistration):
		return "cross_org_registration"
	case errors.Is(err, ErrRegistrationClosed):
		return "registration_closed"
	case errors.Is(err, ErrWindowNotOpen):
		return "window_not_open"
	case errors.Is(err, ErrWindowClosed):
		return "window_closed"
	case errors.Is(err, ErrTeamNotActive):
		return "team_not_active"
	case errors.Is(err, ErrEmptyTeam):
		return "empty_team"
	case errors.Is(err, ErrInvalidStatusTransition):
		return "invalid_status_transition"
	case errors.Is(err, ErrInvalidStatus):
		return "invalid_status"
	default:
		return "internal_error"
	}
}
