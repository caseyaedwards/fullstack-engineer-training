// Package db keeps the uptime monitor's data in Postgres. It applies the
// schema migrations and implements monitor.Store and monitor.ResultStore
// on a pgx connection pool.
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"fsp/examples/week17/your-monitor/internal/obs"
)

// region: connect

// Connect opens a connection pool to the database at url, for example
// postgres://monitor:secret@127.0.0.1:5432/monitor?sslmode=disable, and
// checks that the database answers. pgxpool.NewWithConfig itself connects
// lazily: without the Ping, a wrong password shows up only at the first
// query. Every statement inside a trace becomes a span of that trace.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	cfg.ConnConfig.Tracer = obs.QueryTracer{}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	return pool, nil
}

// endregion: connect
