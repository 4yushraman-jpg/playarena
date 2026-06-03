// Package testutil provides shared integration-test infrastructure.
//
// Each test package that needs a real PostgreSQL database calls SetupTestDB
// from its TestMain. The function starts a postgres:17-alpine testcontainer,
// applies every migration via golang-migrate, creates a pgxpool.Pool, and
// returns a teardown function.
//
// Docker availability:
//
//	INTEGRATION_SKIP_IF_DOCKER_UNAVAILABLE=true  → tests skipped locally (exit 0)
//	(env var absent or false)                    → fatal error so CI pipelines
//	                                               fail loudly instead of passing
//	                                               with zero tests run
package testutil

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	migrations "github.com/4yushraman-jpg/playarena/db/migrations"
)

const (
	testDBName = "playarena_test"
	testDBUser = "testuser"
	testDBPass = "testpass"
)

// SetupTestDB starts a PostgreSQL 17 container, applies all migrations, and
// returns a pool and a teardown function. The caller is responsible for:
//
//	pool, cleanup := testutil.SetupTestDB(m)
//	defer cleanup()
//	os.Exit(m.Run())
//
// If Docker is unavailable and INTEGRATION_SKIP_IF_DOCKER_UNAVAILABLE=true,
// the process exits with code 0 (tests silently skipped — acceptable for local
// dev without Docker). If the env var is absent, the process exits with a
// non-zero code so CI pipelines cannot silently pass with zero tests.
func SetupTestDB(m *testing.M) (*pgxpool.Pool, func()) {
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx,
		"postgres:17-alpine",
		tcpostgres.WithDatabase(testDBName),
		tcpostgres.WithUsername(testDBUser),
		tcpostgres.WithPassword(testDBPass),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		handleDockerUnavailable(m, err)
	}

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = container.Terminate(ctx)
		log.Fatalf("testutil: get connection string: %v", err)
	}

	if err := applyMigrations(connStr); err != nil {
		_ = container.Terminate(ctx)
		log.Fatalf("testutil: apply migrations: %v", err)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		_ = container.Terminate(ctx)
		log.Fatalf("testutil: create pool: %v", err)
	}

	cleanup := func() {
		pool.Close()
		if err := container.Terminate(ctx); err != nil {
			log.Printf("testutil: terminate container: %v", err)
		}
	}

	return pool, cleanup
}

// applyMigrations runs all *.up.sql files from the embedded migrations FS
// against the target database using golang-migrate + pgx/v5 driver.
func applyMigrations(connStr string) error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("create migration source: %w", err)
	}

	// golang-migrate's pgx/v5 driver uses the pgx5:// URL scheme.
	pgx5URL := strings.Replace(connStr, "postgres://", "pgx5://", 1)

	m, err := migrate.NewWithSourceInstance("iofs", src, pgx5URL)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}

// handleDockerUnavailable decides whether to skip (local dev) or fail (CI)
// when the container runtime is unreachable.
func handleDockerUnavailable(m *testing.M, containerErr error) {
	if os.Getenv("INTEGRATION_SKIP_IF_DOCKER_UNAVAILABLE") == "true" {
		fmt.Printf("testutil: skipping integration tests — Docker unavailable (%v)\n", containerErr)
		// Exit 0 with m.Run() not called: go test reports "ok" with no tests run.
		// This is the intended local-dev behaviour when Docker is not running.
		os.Exit(0)
	}
	// CI must not silently pass. A missing container runtime is a pipeline error.
	log.Fatalf("testutil: start postgres container: %v\n"+
		"(set INTEGRATION_SKIP_IF_DOCKER_UNAVAILABLE=true to skip locally)", containerErr)
}
