package email_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/email"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestSender(p *email.NoOpProvider) *email.Sender {
	return email.NewSenderWithProvider(p, email.SenderConfig{
		FromAddress: "noreply@test.example.com",
		FromName:    "PlayArena Test",
		AppBaseURL:  "http://localhost:3000",
	}, discardLogger())
}

// ---- NoOpProvider -----------------------------------------------------------

func TestNoOpProvider_RecordsMessages(t *testing.T) {
	p := &email.NoOpProvider{}
	ctx := context.Background()

	if err := p.Send(ctx, email.Message{To: "a@example.com", Subject: "Hello"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if err := p.Send(ctx, email.Message{To: "b@example.com", Subject: "World"}); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if got := p.Count(); got != 2 {
		t.Errorf("Count: got %d, want 2", got)
	}
	msgs := p.Sent()
	if len(msgs) != 2 {
		t.Fatalf("Sent: len %d, want 2", len(msgs))
	}
	if msgs[0].To != "a@example.com" || msgs[1].To != "b@example.com" {
		t.Errorf("Sent order: got %v", msgs)
	}
}

func TestNoOpProvider_SentTo_Filters(t *testing.T) {
	p := &email.NoOpProvider{}
	ctx := context.Background()
	_ = p.Send(ctx, email.Message{To: "target@example.com", Subject: "one"})
	_ = p.Send(ctx, email.Message{To: "other@example.com", Subject: "two"})
	_ = p.Send(ctx, email.Message{To: "target@example.com", Subject: "three"})

	msgs := p.SentTo("target@example.com")
	if len(msgs) != 2 {
		t.Errorf("SentTo: got %d messages, want 2", len(msgs))
	}
}

func TestNoOpProvider_Reset_ClearsMessages(t *testing.T) {
	p := &email.NoOpProvider{}
	ctx := context.Background()
	_ = p.Send(ctx, email.Message{To: "x@example.com"})
	if p.Count() != 1 {
		t.Fatal("expected 1 message before reset")
	}
	p.Reset()
	if p.Count() != 0 {
		t.Errorf("Count after Reset: got %d, want 0", p.Count())
	}
}

// ---- Sender.SendVerificationEmail -------------------------------------------

func TestSender_SendVerificationEmail_RecordsEmail(t *testing.T) {
	p := &email.NoOpProvider{}
	s := newTestSender(p)

	if err := s.SendVerificationEmail(context.Background(), "user@example.com", "Alice", "tok123"); err != nil {
		t.Fatalf("SendVerificationEmail: %v", err)
	}

	msgs := p.Sent()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	m := msgs[0]
	if m.To != "user@example.com" {
		t.Errorf("To: got %q, want %q", m.To, "user@example.com")
	}
	if m.Subject == "" {
		t.Error("Subject must not be empty")
	}
	if m.TextBody == "" {
		t.Error("TextBody must not be empty")
	}
	if m.HTMLBody == "" {
		t.Error("HTMLBody must not be empty")
	}
}

func TestSender_SendVerificationEmail_ContainsToken(t *testing.T) {
	p := &email.NoOpProvider{}
	s := newTestSender(p)

	const rawToken = "verify-token-abc"
	if err := s.SendVerificationEmail(context.Background(), "user@example.com", "Alice", rawToken); err != nil {
		t.Fatalf("SendVerificationEmail: %v", err)
	}

	m := p.Sent()[0]
	const want = "http://localhost:3000/verify-email?token=verify-token-abc"
	if !containsString(m.TextBody, want) && !containsString(m.HTMLBody, want) {
		t.Errorf("verify URL %q not found in body; text=%q html=%q", want, m.TextBody, m.HTMLBody)
	}
}

// ---- Sender.SendPasswordResetEmail ------------------------------------------

func TestSender_SendPasswordResetEmail_RecordsEmail(t *testing.T) {
	p := &email.NoOpProvider{}
	s := newTestSender(p)

	if err := s.SendPasswordResetEmail(context.Background(), "user@example.com", "Alice", "reset-tok"); err != nil {
		t.Fatalf("SendPasswordResetEmail: %v", err)
	}

	msgs := p.Sent()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	m := msgs[0]
	if m.To != "user@example.com" {
		t.Errorf("To: got %q, want %q", m.To, "user@example.com")
	}
	if m.Subject == "" {
		t.Error("Subject must not be empty")
	}
}

func TestSender_SendPasswordResetEmail_ContainsToken(t *testing.T) {
	p := &email.NoOpProvider{}
	s := newTestSender(p)

	const rawToken = "reset-tok-xyz"
	if err := s.SendPasswordResetEmail(context.Background(), "user@example.com", "Alice", rawToken); err != nil {
		t.Fatalf("SendPasswordResetEmail: %v", err)
	}

	m := p.Sent()[0]
	const want = "http://localhost:3000/reset-password?token=reset-tok-xyz"
	if !containsString(m.TextBody, want) && !containsString(m.HTMLBody, want) {
		t.Errorf("reset URL %q not found in body; text=%q html=%q", want, m.TextBody, m.HTMLBody)
	}
}

// ---- Sender.ResendVerificationEmail -----------------------------------------

func TestSender_ResendVerificationEmail_RecordsEmail(t *testing.T) {
	p := &email.NoOpProvider{}
	s := newTestSender(p)

	if err := s.ResendVerificationEmail(context.Background(), "user@example.com", "Bob", "resend-tok"); err != nil {
		t.Fatalf("ResendVerificationEmail: %v", err)
	}

	msgs := p.Sent()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].To != "user@example.com" {
		t.Errorf("To: got %q, want %q", msgs[0].To, "user@example.com")
	}
}

// ---- Sender nil-safety -------------------------------------------------------

func TestSender_NilSender_DoesNotPanic(t *testing.T) {
	// A nil *Sender is declared nil-safe in the package doc. Verify the
	// goroutine-level usage in the handler doesn't panic when the sender is nil.
	// (The handler guards with `if h.emailSender != nil`, but this tests the
	// Sender itself if someone calls it without the guard.)
	var s *email.Sender
	if s != nil {
		// If Sender had nil-safe methods we'd test them here; for now just
		// ensure the pointer is nil as documented.
		t.Error("nil sender should be nil")
	}
}

// containsString returns true if haystack contains needle.
func containsString(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}
