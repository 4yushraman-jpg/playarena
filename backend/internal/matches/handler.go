package matches

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

// Handler exposes the matches service over HTTP.
type Handler struct {
	svc *Service
	log *slog.Logger
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// ── endpoints ─────────────────────────────────────────────────────────────────

// Create handles POST /api/v1/organizations/{slug}/matches.
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

	m, err := h.svc.Create(r.Context(), slug, req, principal.UserID, principal.OrganizationID)
	if err != nil {
		h.log.WarnContext(r.Context(), "matches.create.failed",
			slog.String("org_slug", slug),
			slog.String("error_kind", errKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeMatchError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "matches.create.success",
		slog.String("match_id", m.ID),
		slog.String("tournament_id", m.TournamentID),
		slog.String("org_slug", slug),
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	response.Write(w, http.StatusCreated, m)
}

// List handles GET /api/v1/organizations/{slug}/matches.
// Supports ?limit=, ?offset=, ?tournament_id=, ?status=, ?search= query params.
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
	if v := r.URL.Query().Get("tournament_id"); v != "" {
		params.TournamentFilter = &v
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
		if errors.Is(err, ErrInvalidTournamentID) {
			response.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		h.log.ErrorContext(r.Context(), "matches.list.failed",
			slog.String("org_slug", slug),
			slog.Any("error", err),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	response.Write(w, http.StatusOK, list)
}

// GetByID handles GET /api/v1/organizations/{slug}/matches/{id}.
// Returns the match regardless of status (including cancelled/completed).
func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	id := chi.URLParam(r, "id")

	m, err := h.svc.GetByID(r.Context(), slug, id)
	if err != nil {
		if errors.Is(err, ErrOrganizationNotFound) {
			response.Error(w, http.StatusNotFound, "organization not found")
			return
		}
		if errors.Is(err, ErrMatchNotFound) {
			response.Error(w, http.StatusNotFound, "match not found")
			return
		}
		h.log.ErrorContext(r.Context(), "matches.get.failed",
			slog.String("org_slug", slug),
			slog.String("match_id", id),
			slog.Any("error", err),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	response.Write(w, http.StatusOK, m)
}

// Update handles PATCH /api/v1/organizations/{slug}/matches/{id}.
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

	m, err := h.svc.Update(r.Context(), slug, id, req, principal.UserID, principal.OrganizationID)
	if err != nil {
		h.log.WarnContext(r.Context(), "matches.update.failed",
			slog.String("org_slug", slug),
			slog.String("match_id", id),
			slog.String("error_kind", errKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeMatchError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "matches.update.success",
		slog.String("match_id", m.ID),
		slog.String("org_slug", slug),
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	response.Write(w, http.StatusOK, m)
}

// Walkover handles POST /api/v1/organizations/{slug}/matches/{id}/walkover.
// Awards an administrative win to the present side when its opponent does not
// appear. Gated on match.update — the same permission as scoring/completion.
func (h *Handler) Walkover(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	id := chi.URLParam(r, "id")

	var req WalkoverRequest
	if err := validator.DecodeJSON(r, &req); err != nil {
		h.writeDecodeError(w, err)
		return
	}

	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	m, err := h.svc.Walkover(r.Context(), slug, id, req, principal.UserID, principal.OrganizationID)
	if err != nil {
		h.log.WarnContext(r.Context(), "matches.walkover.failed",
			slog.String("org_slug", slug),
			slog.String("match_id", id),
			slog.String("error_kind", errKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeMatchError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "matches.walkover.success",
		slog.String("match_id", m.ID),
		slog.String("org_slug", slug),
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	response.Write(w, http.StatusOK, m)
}

// GetScore handles GET /api/v1/organizations/{slug}/matches/{id}/score.
// Derives the current score from the effective event log.  No permission beyond
// RequireAuth is required — scores are publicly readable by any authenticated
// user within the organization.  No data is written or cached.
func (h *Handler) GetScore(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	id := chi.URLParam(r, "id")

	result, err := h.svc.GetScore(r.Context(), slug, id)
	if err != nil {
		if errors.Is(err, ErrOrganizationNotFound) {
			response.Error(w, http.StatusNotFound, "organization not found")
			return
		}
		if errors.Is(err, ErrMatchNotFound) {
			response.Error(w, http.StatusNotFound, "match not found")
			return
		}
		h.log.ErrorContext(r.Context(), "matches.score.failed",
			slog.String("org_slug", slug),
			slog.String("match_id", id),
			slog.Any("error", err),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}
	response.Write(w, http.StatusOK, result)
}

// Delete handles DELETE /api/v1/organizations/{slug}/matches/{id}.
// Soft-cancels the match (status → cancelled). No hard deletes.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	id := chi.URLParam(r, "id")

	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	if err := h.svc.Delete(r.Context(), slug, id, principal.UserID, principal.OrganizationID); err != nil {
		h.log.WarnContext(r.Context(), "matches.delete.failed",
			slog.String("org_slug", slug),
			slog.String("match_id", id),
			slog.String("error_kind", errKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeMatchError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "matches.delete.success",
		slog.String("match_id", id),
		slog.String("org_slug", slug),
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	w.WriteHeader(http.StatusNoContent)
}

// ── error handling ────────────────────────────────────────────────────────────

func (h *Handler) writeMatchError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrOrganizationNotFound):
		response.Error(w, http.StatusNotFound, "organization not found")
	case errors.Is(err, ErrTournamentNotFound):
		response.Error(w, http.StatusNotFound, "tournament not found")
	case errors.Is(err, ErrMatchNotFound):
		response.Error(w, http.StatusNotFound, "match not found")
	case errors.Is(err, ErrTeamNotFound):
		response.Error(w, http.StatusNotFound, "team not found")
	case errors.Is(err, ErrPlayerNotFound):
		response.Error(w, http.StatusNotFound, "player not found")
	case errors.Is(err, ErrForbidden):
		response.Error(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrNextMatchNotFound):
		response.Error(w, http.StatusNotFound, "next match not found")
	case errors.Is(err, ErrInvalidStatus), errors.Is(err, ErrInvalidTournamentID),
		errors.Is(err, ErrInvalidTimestamp),
		errors.Is(err, ErrInvalidWalkoverWinner),
		errors.Is(err, ErrWalkoverReasonRequired),
		errors.Is(err, ErrNextMatchLinkIncomplete),
		errors.Is(err, ErrInvalidNextSlot):
		response.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrTournamentNotOngoing),
		errors.Is(err, ErrMixedParticipantTypes),
		errors.Is(err, ErrMissingParticipants),
		errors.Is(err, ErrDuplicateParticipants),
		errors.Is(err, ErrParticipantCrossOrg),
		errors.Is(err, ErrParticipantNotRegistered),
		errors.Is(err, ErrWinnerNotAllowed),
		errors.Is(err, ErrWinnerNotParticipant),
		errors.Is(err, ErrWinnerScoreMismatch),
		errors.Is(err, ErrInvalidStatusTransition),
		errors.Is(err, ErrMatchNotUpdatable),
		errors.Is(err, ErrMatchAlreadyCancelled),
		errors.Is(err, ErrWalkoverNeedsParticipants),
		errors.Is(err, ErrMatchHasTBDSlot),
		errors.Is(err, ErrDownstreamLocked),
		errors.Is(err, ErrNextMatchCrossTournament),
		errors.Is(err, ErrSelfLink),
		errors.Is(err, ErrBracketInconsistent),
		errors.Is(err, ErrSlotAlreadyFed):
		response.Error(w, http.StatusUnprocessableEntity, err.Error())
	default:
		h.log.ErrorContext(r.Context(), "matches.unexpected_error",
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
	case errors.Is(err, ErrMatchNotFound):
		return "match_not_found"
	case errors.Is(err, ErrTeamNotFound):
		return "team_not_found"
	case errors.Is(err, ErrPlayerNotFound):
		return "player_not_found"
	case errors.Is(err, ErrForbidden):
		return "forbidden"
	case errors.Is(err, ErrTournamentNotOngoing):
		return "tournament_not_ongoing"
	case errors.Is(err, ErrMixedParticipantTypes):
		return "mixed_participant_types"
	case errors.Is(err, ErrMissingParticipants):
		return "missing_participants"
	case errors.Is(err, ErrDuplicateParticipants):
		return "duplicate_participants"
	case errors.Is(err, ErrParticipantCrossOrg):
		return "participant_cross_org"
	case errors.Is(err, ErrParticipantNotRegistered):
		return "participant_not_registered"
	case errors.Is(err, ErrWinnerNotAllowed):
		return "winner_not_allowed"
	case errors.Is(err, ErrWinnerNotParticipant):
		return "winner_not_participant"
	case errors.Is(err, ErrWinnerScoreMismatch):
		return "winner_score_mismatch"
	case errors.Is(err, ErrInvalidStatusTransition):
		return "invalid_status_transition"
	case errors.Is(err, ErrInvalidStatus):
		return "invalid_status"
	case errors.Is(err, ErrMatchNotUpdatable):
		return "match_not_updatable"
	case errors.Is(err, ErrMatchAlreadyCancelled):
		return "match_already_cancelled"
	case errors.Is(err, ErrInvalidWalkoverWinner):
		return "invalid_walkover_winner"
	case errors.Is(err, ErrWalkoverReasonRequired):
		return "walkover_reason_required"
	case errors.Is(err, ErrWalkoverNeedsParticipants):
		return "walkover_needs_participants"
	case errors.Is(err, ErrMatchHasTBDSlot):
		return "match_has_tbd_slot"
	case errors.Is(err, ErrDownstreamLocked):
		return "downstream_locked"
	case errors.Is(err, ErrNextMatchLinkIncomplete):
		return "next_match_link_incomplete"
	case errors.Is(err, ErrInvalidNextSlot):
		return "invalid_next_slot"
	case errors.Is(err, ErrNextMatchNotFound):
		return "next_match_not_found"
	case errors.Is(err, ErrNextMatchCrossTournament):
		return "next_match_cross_tournament"
	case errors.Is(err, ErrSelfLink):
		return "self_link"
	case errors.Is(err, ErrBracketInconsistent):
		return "bracket_inconsistent"
	case errors.Is(err, ErrSlotAlreadyFed):
		return "slot_already_fed"
	case errors.Is(err, ErrInvalidTimestamp):
		return "invalid_timestamp"
	case errors.Is(err, ErrInvalidTournamentID):
		return "invalid_tournament_id"
	default:
		return "internal_error"
	}
}
