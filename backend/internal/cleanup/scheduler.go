// Package cleanup provides a background scheduler that periodically deletes
// expired tokens from the database. It is the single authoritative site for
// all token table pruning: refresh_tokens, email_verification_tokens, and
// password_reset_tokens.
package cleanup

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
)

// Scheduler runs cleanup tasks on a fixed interval.
// Start() launches the background goroutine; Stop() signals it to exit.
// Stop() is safe to call multiple times; only the first call has effect.
type Scheduler struct {
	queries  *db.Queries
	interval time.Duration
	log      *slog.Logger
	done     chan struct{}
}

// New creates a Scheduler. Call Start() to begin cleanup cycles.
func New(queries *db.Queries, interval time.Duration, log *slog.Logger) *Scheduler {
	return &Scheduler{
		queries:  queries,
		interval: interval,
		log:      log,
		done:     make(chan struct{}),
	}
}

// Start launches the background cleanup goroutine. It is a no-op after the
// first call.
func (s *Scheduler) Start() {
	go s.run()
}

// Stop signals the background goroutine to exit. Returns immediately; the
// goroutine exits on its next wakeup. Safe to call multiple times.
func (s *Scheduler) Stop() {
	select {
	case <-s.done:
		// Already stopped.
	default:
		close(s.done)
	}
}

func (s *Scheduler) run() {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.runOnce()
		case <-s.done:
			return
		}
	}
}

// runOnce executes all cleanup queries with a 30-second timeout.
// Each query is independent — a failure in one does not prevent the others.
func (s *Scheduler) runOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cutoff := pgtype.Timestamptz{Time: time.Now(), Valid: true}

	if err := s.queries.DeleteExpiredRefreshTokens(ctx, cutoff); err != nil {
		s.log.Error("cleanup: delete expired refresh tokens", slog.Any("error", err))
	}

	if err := s.queries.DeleteExpiredEmailVerificationTokens(ctx, cutoff); err != nil {
		s.log.Error("cleanup: delete expired email verification tokens", slog.Any("error", err))
	}

	if err := s.queries.DeleteExpiredPasswordResetTokens(ctx, cutoff); err != nil {
		s.log.Error("cleanup: delete expired password reset tokens", slog.Any("error", err))
	}

	s.log.Info("cleanup: cycle complete",
		slog.String("cutoff", cutoff.Time.UTC().Format(time.RFC3339)),
		slog.String("interval", s.interval.String()),
	)
}
