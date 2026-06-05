// Package email provides the email delivery abstraction and concrete provider
// implementations used to send transactional emails (account verification,
// password reset).
//
// Providers:
//
//	ses   — AWS SES v2 (production)
//	smtp  — SMTP with optional STARTTLS (local dev with MailHog)
//	log   — Logs email details via slog without sending (dev default)
//	noop  — Records emails in memory without sending (testing)
//
// Provider selection is driven by Config.EmailProvider. The Sender type is
// the public entry point; all consumers depend on Sender, not Provider.
package email

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/4yushraman-jpg/playarena/internal/platform/config"
)

// Message is an outbound email message. HTMLBody may be empty for providers
// that deliver plain text only.
type Message struct {
	To       string // recipient email address
	ToName   string // recipient display name (optional)
	Subject  string
	TextBody string
	HTMLBody string // empty when the provider is text-only
}

// Provider is the pluggable email delivery backend.
// Implementations must be safe for concurrent use.
type Provider interface {
	Send(ctx context.Context, msg Message) error
}

// SenderConfig holds application-level defaults applied to every outgoing
// message. Fields are sourced from Config at construction time.
type SenderConfig struct {
	FromAddress string // e.g. "noreply@playarena.com"
	FromName    string // e.g. "PlayArena"
	AppBaseURL  string // e.g. "https://app.playarena.com" — for link generation
}

// Sender composes a Provider with application-level defaults and structured
// logging. It is the single entry point for all email delivery in the
// application. Nil-safe: methods are no-ops when the Sender itself is nil.
type Sender struct {
	provider Provider
	cfg      SenderConfig
	log      *slog.Logger
}

// NewSender constructs a Sender by reading cfg.EmailProvider and building the
// appropriate Provider. Returns an error if required configuration for the
// chosen provider is absent.
//
// Provider selection:
//
//	"ses"  — AWS SES v2 (requires AWS credentials or IAM role)
//	"smtp" — SMTP / STARTTLS (local dev with MailHog or similar)
//	"log"  — Logs email content to slog; never sends (development default)
//	""     — Treated as "log"
//	"noop" — Records emails in memory; never sends (testing)
func NewSender(cfg *config.Config, log *slog.Logger) (*Sender, error) {
	sc := SenderConfig{
		FromAddress: cfg.EmailFromAddress,
		FromName:    cfg.EmailFromName,
		AppBaseURL:  cfg.AppBaseURL,
	}

	var provider Provider

	switch cfg.EmailProvider {
	case "ses":
		p, err := newSESProviderImpl(cfg.EmailSESRegion, cfg.EmailSESAccessKey, cfg.EmailSESSecretKey, cfg.EmailFromAddress, cfg.EmailFromName)
		if err != nil {
			return nil, fmt.Errorf("email: SES provider init: %w", err)
		}
		provider = p
	case "smtp":
		provider = newSMTPProviderImpl(cfg.EmailSMTPHost, cfg.EmailSMTPPort, cfg.EmailSMTPUsername, cfg.EmailSMTPPassword, cfg.EmailSMTPTLS, cfg.EmailFromAddress, cfg.EmailFromName)
	case "noop":
		provider = &NoOpProvider{}
	case "log", "":
		provider = &LogProvider{log: log}
	default:
		return nil, fmt.Errorf("email: unknown EMAIL_PROVIDER %q (valid: ses, smtp, log, noop)", cfg.EmailProvider)
	}

	return &Sender{provider: provider, cfg: sc, log: log}, nil
}

// ---- NoOpProvider -----------------------------------------------------------

// NoOpProvider records sent emails in memory without delivering them.
// Intended exclusively for use in tests. Thread-safe.
type NoOpProvider struct {
	mu   sync.Mutex
	sent []Message
}

// Send appends msg to the internal slice. Always returns nil.
func (n *NoOpProvider) Send(_ context.Context, msg Message) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sent = append(n.sent, msg)
	return nil
}

// Sent returns a copy of all recorded messages in send order. Thread-safe.
func (n *NoOpProvider) Sent() []Message {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]Message, len(n.sent))
	copy(out, n.sent)
	return out
}

// SentTo returns all messages addressed to the given email address.
func (n *NoOpProvider) SentTo(addr string) []Message {
	n.mu.Lock()
	defer n.mu.Unlock()
	var out []Message
	for _, m := range n.sent {
		if m.To == addr {
			out = append(out, m)
		}
	}
	return out
}

// Reset clears all recorded messages. Call between subtests to isolate state.
func (n *NoOpProvider) Reset() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sent = n.sent[:0]
}

// Count returns the number of messages sent so far. Thread-safe.
func (n *NoOpProvider) Count() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.sent)
}

// ---- LogProvider ------------------------------------------------------------

// LogProvider logs email details via slog without sending anything.
// Used as the default provider in development when no email transport is
// configured. Satisfies the Provider interface.
type LogProvider struct {
	log *slog.Logger
}

// Send logs the email fields and returns nil.
func (l *LogProvider) Send(_ context.Context, msg Message) error {
	l.log.Info("email.log_provider.send",
		slog.String("to", msg.To),
		slog.String("subject", msg.Subject),
		slog.Int("text_len", len(msg.TextBody)),
		slog.Int("html_len", len(msg.HTMLBody)),
	)
	return nil
}

// NewSenderWithProvider constructs a Sender using an explicitly supplied
// Provider. Intended for tests that need to inspect sent emails via a
// NoOpProvider without going through the config-driven NewSender factory.
func NewSenderWithProvider(provider Provider, cfg SenderConfig, log *slog.Logger) *Sender {
	return &Sender{provider: provider, cfg: cfg, log: log}
}
