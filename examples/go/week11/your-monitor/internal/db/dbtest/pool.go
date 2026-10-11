package dbtest

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// region: pool

// Pool opens a connection pool on url, applies migrate, and closes the
// pool when the test ends. Pass URL(t) and db.Migrate: the tests of
// Week 9 get an empty schema with every migration in it. migrate is a
// parameter, not an import of package db, because the tests of package db
// use dbtest too, and Go refuses an import cycle.
func Pool(t testing.TB, url string, migrate func(context.Context, *pgxpool.Pool) error) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 20 // the concurrency tests hold one connection per worker
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := migrate(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
	return pool
}

// endregion: pool

// Exec runs one SQL statement and fails the test on error.
func Exec(t testing.TB, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

// Count runs a query that returns one number and returns it.
func Count(t testing.TB, pool *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return n
}
