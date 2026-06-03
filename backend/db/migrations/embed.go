package migrations

import "embed"

// FS holds all migration files (*.up.sql and *.down.sql) embedded at compile
// time. Imported by the test-infrastructure package to run golang-migrate
// without requiring the migrations directory to be present on disk at runtime.
//
//go:embed *.sql
var FS embed.FS
