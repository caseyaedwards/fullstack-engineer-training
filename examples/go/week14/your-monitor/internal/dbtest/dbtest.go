// Package dbtest gives each test its own empty, migrated schema in the
// database that DATABASE_URL names, and drops it when the test ends.
// Tests in different packages run at the same time, and their own schema
// keeps one test's jobs out of another test's queue.
package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"fsp/examples/week14/your-monitor/internal/db"
)

// New returns a pool whose connections use a new schema with every
// migration applied. Without DATABASE_URL, the test fails when CI=true
// and is skipped otherwise.
func New(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, _ := NewWithURL(t)
	return pool
}

// NewWithURL is New, and also returns a connection string for the same
// schema, for a program that the test starts.
func NewWithURL(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	// region: ci
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		if os.Getenv("CI") == "true" {
			t.Fatal("DATABASE_URL is not set; CI must provide a Postgres database")
		}
		t.Skip("DATABASE_URL is not set; set it to run the Postgres tests")
	}
	// endregion: ci
	ctx := context.Background()

	b := make([]byte, 6)
	_, _ = rand.Read(b) // crypto/rand.Read never returns an error since Go 1.24
	schema := "test_" + hex.EncodeToString(b)

	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	// Every connection of the pool finds its tables in the new schema.
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.MaxConns = 20
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		conn, err := pgx.Connect(context.Background(), url)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close(context.Background())
		if _, err := conn.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	// pgx passes a query parameter it does not know on to the server as a
	// setting, the way it passes search_path here.
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	return pool, url + sep + "search_path=" + schema
}

// Exec runs one SQL statement and fails the test on error.
func Exec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

// Count runs a query that returns one number and returns it.
func Count(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return n
}
