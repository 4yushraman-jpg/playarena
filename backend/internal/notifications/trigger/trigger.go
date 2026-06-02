// Package trigger provides a lightweight helper for writing notification outbox
// entries inside domain transactions.
//
// Domain repositories (matches, tournaments, tournament_registrations) call
// WriteOutboxEntry within their own transactions (before Commit) to atomically
// record that a domain event occurred. The notifications service DrainOutbox
// then converts these entries into actual notifications after the transaction
// commits.
//
// No business logic lives here: this package is purely a DB write helper.
package trigger

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
)

// OutboxParams contains all data needed to write one outbox entry.
type OutboxParams struct {
	OrganizationID pgtype.UUID
	EventType      db.NotificationEventType
	// ActorID is the user who performed the action. Pass pgtype.UUID{} for system events.
	ActorID    pgtype.UUID
	EntityType string
	EntityID   pgtype.UUID
	// Payload is event-specific structured data (e.g., old/new status).
	// nil is treated as an empty JSON object.
	Payload map[string]any
}

// WriteOutboxEntry inserts one notification_outbox row using the provided
// transaction-scoped Queries object (qtx). This must be called INSIDE an
// existing domain transaction so that the outbox entry is written atomically
// with the domain operation.
//
// The caller is responsible for passing a transaction-scoped qtx
// (obtained via queries.WithTx(tx)).
func WriteOutboxEntry(ctx context.Context, qtx *db.Queries, p OutboxParams) error {
	payload, err := marshalPayload(p.Payload)
	if err != nil {
		return err
	}

	_, err = qtx.CreateNotificationOutboxEntry(ctx, db.CreateNotificationOutboxEntryParams{
		OrganizationID: p.OrganizationID,
		EventType:      p.EventType,
		ActorID:        p.ActorID,
		EntityType:     p.EntityType,
		EntityID:       p.EntityID,
		Payload:        payload,
	})
	return err
}

func marshalPayload(m map[string]any) ([]byte, error) {
	if len(m) == 0 {
		return []byte("{}"), nil
	}
	return json.Marshal(m)
}
