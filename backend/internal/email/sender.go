package email

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// SendVerificationEmail renders and delivers the email-verification message.
// The rawToken is embedded in a link; it is never logged.
//
// Failure is returned to the caller so the handler can log and continue —
// account creation must not fail because email delivery failed.
func (s *Sender) SendVerificationEmail(ctx context.Context, toEmail, toName, rawToken string) error {
	verifyURL := s.cfg.AppBaseURL + "/verify-email?token=" + rawToken
	subject, textBody, htmlBody, err := renderVerifyEmail(verifyEmailData{
		UserName:  nameOrEmail(toName, toEmail),
		VerifyURL: verifyURL,
		ExpiresIn: "24 hours",
		AppName:   s.cfg.FromName,
	})
	if err != nil {
		return fmt.Errorf("email: render verify_email template: %w", err)
	}
	return s.send(ctx, Message{
		To:       toEmail,
		ToName:   toName,
		Subject:  subject,
		TextBody: textBody,
		HTMLBody: htmlBody,
	}, "verify_email")
}

// SendPasswordResetEmail renders and delivers the password-reset message.
// The rawToken is embedded in a link; it is never logged.
//
// This is called from a detached goroutine in the ForgotPassword handler so
// that email delivery latency does not affect the HTTP response timing (which
// is important for enumeration resistance). The context passed here must be
// context.Background(), not the request context.
func (s *Sender) SendPasswordResetEmail(ctx context.Context, toEmail, toName, rawToken string) error {
	resetURL := s.cfg.AppBaseURL + "/reset-password?token=" + rawToken
	subject, textBody, htmlBody, err := renderPasswordReset(resetPasswordData{
		UserName:  nameOrEmail(toName, toEmail),
		ResetURL:  resetURL,
		ExpiresIn: "1 hour",
		AppName:   s.cfg.FromName,
	})
	if err != nil {
		return fmt.Errorf("email: render password_reset template: %w", err)
	}
	return s.send(ctx, Message{
		To:       toEmail,
		ToName:   toName,
		Subject:  subject,
		TextBody: textBody,
		HTMLBody: htmlBody,
	}, "password_reset")
}

// ResendVerificationEmail is identical to SendVerificationEmail but logged
// under a distinct template name so resend events can be tracked separately.
func (s *Sender) ResendVerificationEmail(ctx context.Context, toEmail, toName, rawToken string) error {
	verifyURL := s.cfg.AppBaseURL + "/verify-email?token=" + rawToken
	subject, textBody, htmlBody, err := renderVerifyEmail(verifyEmailData{
		UserName:  nameOrEmail(toName, toEmail),
		VerifyURL: verifyURL,
		ExpiresIn: "24 hours",
		AppName:   s.cfg.FromName,
	})
	if err != nil {
		return fmt.Errorf("email: render verify_email template (resend): %w", err)
	}
	return s.send(ctx, Message{
		To:       toEmail,
		ToName:   toName,
		Subject:  subject,
		TextBody: textBody,
		HTMLBody: htmlBody,
	}, "resend_verify_email")
}

// SendNotificationEmail renders and delivers a notification event email.
// Called by the EmailWorker for each pending email channel notification row.
func (s *Sender) SendNotificationEmail(ctx context.Context, toEmail, toName, eventLabel, notificationsURL string) error {
	subject, textBody, htmlBody, err := renderNotificationEvent(notificationEventData{
		UserName:         nameOrEmail(toName, toEmail),
		EventLabel:       eventLabel,
		NotificationsURL: notificationsURL,
		AppName:          s.cfg.FromName,
	})
	if err != nil {
		return fmt.Errorf("email: render notification_event template: %w", err)
	}
	return s.send(ctx, Message{
		To:       toEmail,
		ToName:   toName,
		Subject:  subject,
		TextBody: textBody,
		HTMLBody: htmlBody,
	}, "notification_event")
}

// send delivers msg via the configured provider and emits a structured log
// entry. The template name is used for log attribution only — it is never
// sent to the provider.
func (s *Sender) send(ctx context.Context, msg Message, template string) error {
	start := time.Now()
	err := s.provider.Send(ctx, msg)
	ms := time.Since(start).Milliseconds()

	if err != nil {
		s.log.ErrorContext(ctx, "email.send.failed",
			slog.String("template", template),
			slog.Int64("duration_ms", ms),
			slog.String("error", err.Error()),
		)
		return err
	}

	s.log.InfoContext(ctx, "email.send.success",
		slog.String("template", template),
		slog.Int64("duration_ms", ms),
	)
	return nil
}

// nameOrEmail returns name if non-empty, otherwise the email address.
// Used to populate the UserName field in templates when a display name is
// not available.
func nameOrEmail(name, email string) string {
	if name != "" {
		return name
	}
	return email
}
