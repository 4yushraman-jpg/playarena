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

	// Strip the verification token in production — it must only be delivered
	// via email (H2 fix). In development it is returned for testing convenience.
	if !h.config.IsDevelopment() {
		resp.VerificationToken = ""
	}
	response.Write(w, http.StatusCreated, resp)
}

// VerifyEmail handles GET /api/v1/auth/verify-email?token=<raw_token>.
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

// ForgotPassword handles POST /api/v1/auth/forgot-password.
//
// Always returns HTTP 200 with the same message body regardless of whether the
// email address is registered. This prevents user-enumeration: an attacker
// cannot distinguish "no account" from "token created" by observing the HTTP
// response.
//
// In development the raw reset token is included in the response body for
// testing without an email transport. In production the field is stripped;
// the token is delivered only via email.
func (h *Handler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req ForgotPasswordRequest
	if err := validator.DecodeJSON(r, &req); err != nil {
		h.writeDecodeError(w, err)
		return
	}

	resp, err := h.svc.ForgotPassword(r.Context(), req)
	if err != nil {
		h.log.ErrorContext(r.Context(), "auth.forgot_password.internal_error",
			slog.Any("error", err),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		// Return the generic success message even on internal errors — do not
		// leak whether the operation failed internally.
		response.Write(w, http.StatusOK, ForgotPasswordResponse{
			Message: "if the email is registered, a password reset link has been sent",
		})
		return
	}

	// Strip the token in production — it must only reach the user via email.
	if !h.config.IsDevelopment() {
		resp.ResetToken = ""
	}

	response.Write(w, http.StatusOK, resp)
}

// ResetPassword handles POST /api/v1/auth/reset-password.
//
// Validates the reset token, updates the password, revokes all active
// sessions, and writes an audit record — all atomically. On success all
// existing sessions are invalidated; the client must log in again.
func (h *Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req ResetPasswordRequest
	if err := validator.DecodeJSON(r, &req); err != nil {
		h.writeDecodeError(w, err)
		return
	}

	if err := h.svc.ResetPassword(r.Context(), req); err != nil {
		h.log.WarnContext(r.Context(), "auth.reset_password.failed",
			slog.String("kind", errorKind(err)),
			slog.String("request_id", chimw.GetReqID(r.Context())),
		)
		h.writeAuthError(w, r, err)
		return
	}

	h.log.InfoContext(r.Context(), "auth.reset_password.success",
		slog.String("request_id", chimw.GetReqID(r.Context())),
	)
	response.Write(w, http.StatusOK, struct {
		Message string `json:"message"`
	}{Message: "password reset successfully; all sessions have been revoked"})
}

// AdminOnly handles GET /api/v1/auth/admin-only.
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
	case errors.Is(err, ErrResetTokenInvalid):
		response.Error(w, http.StatusBadRequest, "invalid password reset token")
	case errors.Is(err, ErrResetTokenExpired):
		response.Error(w, http.StatusBadRequest, "password reset token has expired")
	case errors.Is(err, ErrResetTokenUsed):
		response.Error(w, http.StatusBadRequest, "password reset token has already been used")
	case errors.Is(err, ErrPasswordTooLong):
		response.Error(w, http.StatusUnprocessableEntity, "password exceeds maximum length")
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

// errorKind returns a stable loggable string identifying the error type.
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
	case errors.Is(err, ErrResetTokenInvalid):
		return "reset_token_invalid"
	case errors.Is(err, ErrResetTokenExpired):
		return "reset_token_expired"
	case errors.Is(err, ErrResetTokenUsed):
		return "reset_token_used"
	case errors.Is(err, ErrPasswordTooLong):
		return "password_too_long"
	case errors.Is(err, ErrOrganizationNotFound):
		return "organization_not_found"
	default:
		return "internal_error"
	}
}
