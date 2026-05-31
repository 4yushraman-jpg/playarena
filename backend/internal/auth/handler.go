package auth

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/netip"

	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/4yushraman-jpg/playarena/internal/platform/config"
	"github.com/4yushraman-jpg/playarena/internal/platform/response"
	"github.com/4yushraman-jpg/playarena/internal/platform/validator"
)

// Handler exposes the auth service over HTTP.
type Handler struct {
	svc    *Service
	config *config.Config
	log    *slog.Logger
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service, cfg *config.Config, log *slog.Logger) *Handler {
	return &Handler{svc: svc, config: cfg, log: log}
}

// ---- endpoints --------------------------------------------------------------

// Login handles POST /api/v1/auth/login.
//
// On success:           200 with access + refresh tokens
// Multi-org selection:  409 with organization list (client must re-submit with organization_id)
// Bad credentials:      401
// Account blocked:      403
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := validator.DecodeJSON(r, &req); err != nil {
		h.writeDecodeError(w, err)
		return
	}

	resp, err := h.svc.Login(r.Context(), req, extractIP(r), extractUserAgent(r))
	if err != nil {
		h.log.WarnContext(r.Context(), "auth.login.failed",
			slog.String("kind", errorKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeAuthError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "auth.login.success",
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	response.Write(w, http.StatusOK, resp)
}

// Refresh handles POST /api/v1/auth/refresh.
//
// Always rotates the refresh token: the client must store the new one.
// On success:       200 with new access + refresh tokens
// Invalid token:    401
// Token reuse:      401 (all sessions revoked)
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := validator.DecodeJSON(r, &req); err != nil {
		h.writeDecodeError(w, err)
		return
	}

	resp, err := h.svc.Refresh(r.Context(), req, extractIP(r), extractUserAgent(r))
	if err != nil {
		h.log.WarnContext(r.Context(), "auth.refresh.failed",
			slog.String("kind", errorKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeAuthError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "auth.refresh.success",
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	response.Write(w, http.StatusOK, resp)
}

// Logout handles POST /api/v1/auth/logout.
//
// Revokes the presented refresh token. The corresponding access token continues
// to be valid until it expires naturally (15-minute window). No authentication
// middleware is required — the refresh token itself proves identity.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	var req LogoutRequest
	if err := validator.DecodeJSON(r, &req); err != nil {
		h.writeDecodeError(w, err)
		return
	}

	if err := h.svc.Logout(r.Context(), req.RefreshToken); err != nil {
		h.log.WarnContext(r.Context(), "auth.logout.failed",
			slog.String("kind", errorKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeAuthError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "auth.logout",
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	response.Write(w, http.StatusOK, struct {
		Message string `json:"message"`
	}{Message: "logged out"})
}

// Me handles GET /api/v1/auth/me.
//
// Protected by RequireAuth middleware. Combines DB profile data with the
// org context and role already present in the validated access token.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	principal := GetAuthUser(r.Context())
	if principal == nil {
		// Should never happen — RequireAuth runs before this handler.
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	profile, err := h.svc.Me(r.Context(), principal)
	if err != nil {
		h.writeAuthError(w, r, err)
		return
	}

	response.Write(w, http.StatusOK, meResponse{
		ID:             profile.ID,
		Email:          profile.Email,
		Username:       profile.Username,
		FullName:       profile.FullName,
		Status:         profile.Status,
		Role:           principal.Role,
		OrganizationID: principal.OrganizationID,
	})
}

// ---- response types ---------------------------------------------------------

// orgRequiredBody is the response sent when a multi-org user must specify
// organization_id before a token can be issued.
type orgRequiredBody struct {
	Error         string       `json:"error"`
	Code          string       `json:"code"`
	Organizations []OrgSummary `json:"organizations"`
}

// meResponse is the combined DB profile + JWT-context representation.
type meResponse struct {
	ID             string `json:"id"`
	Email          string `json:"email"`
	Username       string `json:"username"`
	FullName       string `json:"full_name"`
	Status         string `json:"status"`
	Role           string `json:"role"`
	OrganizationID string `json:"organization_id"`
}

// ---- error mapping ----------------------------------------------------------

// writeAuthError maps domain errors to HTTP responses.
//
// Mapping (per task specification):
//
//	ErrInvalidCredentials      → 401
//	ErrInvalidToken            → 401
//	ErrExpiredToken            → 401
//	ErrRevokedToken            → 401
//	ErrTokenReuse              → 401 (all sessions revoked)
//	ErrUserSuspended           → 403
//	ErrUserInactive            → 403
//	ErrUserPendingVerification → 403
//	ErrOrganizationRequired    → 409 with org list
//	ErrOrganizationNotFound    → 422
//	anything else              → 500
func (h *Handler) writeAuthError(w http.ResponseWriter, r *http.Request, err error) {
	// ErrOrganizationRequired is a struct error carrying the org list.
	var orgReq *ErrOrganizationRequired
	if errors.As(err, &orgReq) {
		response.Write(w, http.StatusConflict, orgRequiredBody{
			Error:         "organization_id is required",
			Code:          "organization_required",
			Organizations: orgReq.Organizations,
		})
		return
	}

	switch {
	case errors.Is(err, ErrInvalidCredentials):
		response.Error(w, http.StatusUnauthorized, "invalid credentials")
	case errors.Is(err, ErrInvalidToken),
		errors.Is(err, ErrExpiredToken),
		errors.Is(err, ErrRevokedToken),
		errors.Is(err, ErrTokenReuse):
		response.Error(w, http.StatusUnauthorized, "unauthorized")
	case errors.Is(err, ErrUserSuspended):
		response.Error(w, http.StatusForbidden, "account suspended")
	case errors.Is(err, ErrUserInactive):
		response.Error(w, http.StatusForbidden, "account inactive")
	case errors.Is(err, ErrUserPendingVerification):
		response.Error(w, http.StatusForbidden, "email address not verified")
	case errors.Is(err, ErrOrganizationNotFound):
		response.Error(w, http.StatusUnprocessableEntity, "organization not found or access denied")
	default:
		h.log.ErrorContext(r.Context(), "auth.unexpected_error",
			slog.Any("error", err),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		response.Error(w, http.StatusInternalServerError, "internal server error")
	}
}

// writeDecodeError writes a 400 response for body decoding or validation failures.
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

// ---- request helpers --------------------------------------------------------

// extractIP parses the client IP from r.RemoteAddr, which chi's RealIP
// middleware has already populated from X-Forwarded-For / X-Real-IP.
func extractIP(r *http.Request) *netip.Addr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return nil
	}
	return &addr
}

// extractUserAgent returns a pointer to the User-Agent header value,
// or nil when the header is absent.
func extractUserAgent(r *http.Request) *string {
	if ua := r.Header.Get("User-Agent"); ua != "" {
		return &ua
	}
	return nil
}

// errorKind returns a stable, loggable string identifying the error type
// without exposing the error message, which may contain sensitive context.
func errorKind(err error) string {
	var orgReq *ErrOrganizationRequired
	switch {
	case errors.As(err, &orgReq):
		return "organization_required"
	case errors.Is(err, ErrInvalidCredentials):
		return "invalid_credentials"
	case errors.Is(err, ErrUserSuspended):
		return "user_suspended"
	case errors.Is(err, ErrUserInactive):
		return "user_inactive"
	case errors.Is(err, ErrUserPendingVerification):
		return "email_not_verified"
	case errors.Is(err, ErrInvalidToken):
		return "invalid_token"
	case errors.Is(err, ErrExpiredToken):
		return "expired_token"
	case errors.Is(err, ErrRevokedToken):
		return "revoked_token"
	case errors.Is(err, ErrTokenReuse):
		return "token_reuse_detected"
	case errors.Is(err, ErrOrganizationNotFound):
		return "organization_not_found"
	default:
		return "internal_error"
	}
}
