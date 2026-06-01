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
// Protected by RequireAuth middleware.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	principal := GetAuthUser(r.Context())
	if principal == nil {
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

// Register handles POST /api/v1/auth/register.
//
// Creates a new user with status pending_verification and returns a
// verification token. In production, remove verification_token from the
// response and deliver it only via email.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := validator.DecodeJSON(r, &req); err != nil {
		h.writeDecodeError(w, err)
		return
	}

	resp, err := h.svc.Register(r.Context(), req)
	if err != nil {
		h.log.WarnContext(r.Context(), "auth.register.failed",
			slog.String("kind", errorKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeAuthError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "auth.register.success",
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)

	// H2 fix: strip the verification token in production.
	// In development it is returned for testing convenience (no email transport needed).
	// In production the raw token must only be delivered via email — returning it in
	// the API response allows any interceptor to bypass email verification entirely.
	if !h.config.IsDevelopment() {
		resp.VerificationToken = "" // json:"...,omitempty" ensures the field is absent
	}
	response.Write(w, http.StatusCreated, resp)
}

// VerifyEmail handles GET /api/v1/auth/verify-email?token=<raw_token>.
//
// Consumes the single-use verification token, activates the account, and
// returns a plain success message. The client should redirect to login.
func (h *Handler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	rawToken := r.URL.Query().Get("token")
	if rawToken == "" {
		response.Error(w, http.StatusBadRequest, "token query parameter is required")
		return
	}

	if err := h.svc.VerifyEmail(r.Context(), rawToken); err != nil {
		h.log.WarnContext(r.Context(), "auth.verify_email.failed",
			slog.String("kind", errorKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeAuthError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "auth.verify_email.success",
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	response.Write(w, http.StatusOK, struct {
		Message string `json:"message"`
	}{Message: "email verified successfully"})
}

// AdminOnly handles GET /api/v1/auth/admin-only.
//
// This is a demonstration endpoint that requires RequireAuth +
// RequirePermission("role.assign") in the middleware chain. It exercises
// the authorization layer without doing real business logic.
//
// Response: the caller's principal (user_id, email, role, org_id).
func (h *Handler) AdminOnly(w http.ResponseWriter, r *http.Request) {
	principal := GetAuthUser(r.Context())
	if principal == nil {
		response.Error(w, http.StatusUnauthorized, "authorization required")
		return
	}

	response.Write(w, http.StatusOK, struct {
		Message        string `json:"message"`
		UserID         string `json:"user_id"`
		Email          string `json:"email"`
		Role           string `json:"role"`
		OrganizationID string `json:"organization_id"`
	}{
		Message:        "access granted",
		UserID:         principal.UserID,
		Email:          principal.Email,
		Role:           principal.Role,
		OrganizationID: principal.OrganizationID,
	})
}

// ---- response types ---------------------------------------------------------

type orgRequiredBody struct {
	Error         string       `json:"error"`
	Code          string       `json:"code"`
	Organizations []OrgSummary `json:"organizations"`
}

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
//	ErrInvalidCredentials        → 401
//	ErrInvalidToken / Expired /
//	  Revoked / TokenReuse       → 401
//	ErrUserSuspended             → 403
//	ErrUserInactive              → 403
//	ErrUserPendingVerification   → 403
//	ErrEmailAlreadyRegistered    → 409
//	ErrUsernameAlreadyTaken      → 409
//	ErrOrganizationRequired      → 409 (with org list)
//	ErrVerificationTokenInvalid  → 400
//	ErrVerificationTokenExpired  → 400
//	ErrVerificationTokenUsed     → 400
//	ErrOrganizationNotFound      → 422
//	anything else                → 500
func (h *Handler) writeAuthError(w http.ResponseWriter, r *http.Request, err error) {
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
	case errors.Is(err, ErrEmailAlreadyRegistered):
		response.Error(w, http.StatusConflict, "email address is already registered")
	case errors.Is(err, ErrUsernameAlreadyTaken):
		response.Error(w, http.StatusConflict, "username is already taken")
	case errors.Is(err, ErrVerificationTokenInvalid):
		response.Error(w, http.StatusBadRequest, "invalid verification token")
	case errors.Is(err, ErrVerificationTokenExpired):
		response.Error(w, http.StatusBadRequest, "verification token has expired")
	case errors.Is(err, ErrVerificationTokenUsed):
		response.Error(w, http.StatusBadRequest, "verification token has already been used")
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

// writeDecodeError writes a 400 response for body decode or validation failures.
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

func extractUserAgent(r *http.Request) *string {
	if ua := r.Header.Get("User-Agent"); ua != "" {
		return &ua
	}
	return nil
}

// errorKind returns a stable loggable string identifying the error type
// without exposing the message contents.
func errorKind(err error) string {
	var orgReq *ErrOrganizationRequired
	switch {
	case errors.As(err, &orgReq):
		return "organization_required"
	case errors.Is(err, ErrInvalidCredentials):
		return "invalid_credentials"
	case errors.Is(err, ErrEmailAlreadyRegistered):
		return "email_already_registered"
	case errors.Is(err, ErrUsernameAlreadyTaken):
		return "username_already_taken"
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
	case errors.Is(err, ErrVerificationTokenInvalid):
		return "verification_token_invalid"
	case errors.Is(err, ErrVerificationTokenExpired):
		return "verification_token_expired"
	case errors.Is(err, ErrVerificationTokenUsed):
		return "verification_token_used"
	case errors.Is(err, ErrOrganizationNotFound):
		return "organization_not_found"
	default:
		return "internal_error"
	}
}
