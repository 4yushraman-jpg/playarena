package notifications

import "github.com/jackc/pgx/v5/pgtype"

// ListParams carries pagination options for the notification list endpoint.
type ListParams struct {
	Limit  int32
	Offset int32
}

const (
	DefaultListLimit int32 = 50
	MaxListLimit     int32 = 200
)

// outboxParams is the parameter set written inside domain transactions.
// It is declared here so the trigger package can import it without a circular
// dependency; the trigger package imports db types directly.
type outboxKey struct {
	OrganizationID pgtype.UUID
	EntityType     string
	EntityID       pgtype.UUID
}
