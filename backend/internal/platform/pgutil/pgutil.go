// Package pgutil provides shared PostgreSQL / pgx helper functions used across
// all domain modules. Centralising these prevents copy-paste drift and keeps
// the domain packages free of boilerplate pgtype wrangling.
package pgutil

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// UUIDToString renders a pgtype.UUID as a canonical lower-case hyphenated UUID
// string (xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx).
// Returns an empty string when the UUID is NULL (Valid == false).
func UUIDToString(uid pgtype.UUID) string {
	if !uid.Valid {
		return ""
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		uid.Bytes[0:4],
		uid.Bytes[4:6],
		uid.Bytes[6:8],
		uid.Bytes[8:10],
		uid.Bytes[10:16],
	)
}

// ParseUUID parses a UUID string into a pgtype.UUID.
// Returns an error when s is empty or not a valid UUID.
func ParseUUID(s string) (pgtype.UUID, error) {
	if s == "" {
		return pgtype.UUID{}, fmt.Errorf("pgutil: empty UUID string")
	}
	uid := pgtype.UUID{}
	if err := uid.Scan(s); err != nil {
		return pgtype.UUID{}, fmt.Errorf("pgutil: invalid UUID %q: %w", s, err)
	}
	return uid, nil
}

// ParseOptionalUUID parses an optional UUID string.
// An empty string or any parse failure returns pgtype.UUID{Valid: false},
// which maps to SQL NULL in parameterised queries.
func ParseOptionalUUID(s string) pgtype.UUID {
	if s == "" {
		return pgtype.UUID{}
	}
	uid := pgtype.UUID{}
	if err := uid.Scan(s); err != nil {
		return pgtype.UUID{}
	}
	return uid
}

// IsUniqueViolation reports whether err is a PostgreSQL unique-constraint
// violation (SQLSTATE 23505) for the named constraint.
func IsUniqueViolation(err error, constraintName string) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && pgErr.ConstraintName == constraintName
	}
	return false
}
