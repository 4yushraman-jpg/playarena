package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/4yushraman-jpg/playarena/internal/platform/config"
)

// New creates a *pgxpool.Pool configured for production use.
// It verifies database connectivity before returning.
// The caller must call pool.Close() when the pool is no longer needed.
func New(cfg *config.Config) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("database: parse url: %w", err)
	}

	// Production-safe pool defaults.
	// Tune these via DATABASE_URL query parameters or config once load
	// characteristics are measured in staging.
	poolCfg.MaxConns = 25
	poolCfg.MinConns = 2
	poolCfg.MaxConnLifetime = 1 * time.Hour
	poolCfg.MaxConnIdleTime = 30 * time.Minute
	poolCfg.HealthCheckPeriod = 1 * time.Minute
	poolCfg.ConnConfig.ConnectTimeout = 5 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("database: create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database: ping failed (is PostgreSQL running?): %w", err)
	}

	return pool, nil
}

// Health reports whether the pool can reach the database.
// Uses a short 2-second timeout so health check endpoints stay fast.
// Safe to call from concurrent goroutines.
func Health(ctx context.Context, pool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return pool.Ping(ctx)
}
