package fixturegen

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

// Handler exposes fixture generation over HTTP.
type Handler struct {
	svc *Service
	log *slog.Logger
}

func NewHandler(svc *Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// Generate handles POST .../tournaments/{id}/fixtures/generate.
func (h *Handler) Generate(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	id := chi.URLParam(r, "id")

	var req GenerateRequest
	if err := validator.DecodeJSON(r, &req); err != nil {
		h.writeDecodeError(w, err)
		return
	}
	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	resp, err := h.svc.Generate(r.Context(), slug, id, req, principal.UserID, principal.OrganizationID)
	if err != nil {
		h.log.WarnContext(r.Context(), "fixtures.generate.failed",
			slog.String("org_slug", slug), slog.String("tournament_id", id),
			slog.String("error", err.Error()),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeError(w, r, err)
		return
	}
	status := http.StatusOK
	if !resp.DryRun {
		status = http.StatusCreated
		h.log.InfoContext(r.Context(), "fixtures.generate.success",
			slog.String("tournament_id", id), slog.Int("generated", resp.Generated),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
	}
	response.Write(w, status, resp)
}

// ResolveQualifiers handles POST .../tournaments/{id}/fixtures/resolve-qualifiers.
func (h *Handler) ResolveQualifiers(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	id := chi.URLParam(r, "id")

	principal := auth.GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}
	resp, err := h.svc.ResolveQualifiers(r.Context(), slug, id, principal.UserID, principal.OrganizationID)
	if err != nil {
		h.log.WarnContext(r.Context(), "fixtures.resolve.failed",
			slog.String("tournament_id", id), slog.String("error", err.Error()),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeError(w, r, err)
		return
	}
	response.Write(w, http.StatusOK, resp)
}

// GenerationInfo handles GET .../tournaments/{id}/fixtures/generation.
func (h *Handler) GenerationInfo(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	id := chi.URLParam(r, "id")

	info, err := h.svc.GetGenerationInfo(r.Context(), slug, id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	response.Write(w, http.StatusOK, info)
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrOrganizationNotFound):
		response.Error(w, http.StatusNotFound, "organization not found")
	case errors.Is(err, ErrTournamentNotFound):
		response.Error(w, http.StatusNotFound, "tournament not found")
	case errors.Is(err, ErrForbidden):
		response.Error(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrInvalidTournamentID),
		errors.Is(err, ErrInvalidFormat),
		errors.Is(err, ErrInvalidSeedStrategy):
		response.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrNotRegistrationClosed),
		errors.Is(err, ErrFixturesExist),
		errors.Is(err, ErrRegistrationsChanged),
		errors.Is(err, ErrTooFewParticipants),
		errors.Is(err, ErrInvalidGroupConfig),
		errors.Is(err, ErrNotGroupKnockout),
		errors.Is(err, ErrTournamentNotOngoing),
		errors.Is(err, ErrGroupStageIncomplete),
		errors.Is(err, ErrNoQualifierMatches),
		errors.Is(err, ErrQualifierUnavailable):
		response.Error(w, http.StatusUnprocessableEntity, err.Error())
	default:
		h.log.ErrorContext(r.Context(), "fixtures.unexpected_error",
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
		}{Error: "validation failed", Fields: ve.Fields})
		return
	}
	response.Error(w, http.StatusBadRequest, err.Error())
}
