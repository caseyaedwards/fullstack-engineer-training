// Package dbtest gives each test its own empty schema in the Postgres
// database that DATABASE_URL names.
package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// region: url

// URL creates an empty schema for this test and returns DATABASE_URL with
// search_path set to that schema. Tables that the test creates without a
// schema name land in it, and t.Cleanup drops it with everything inside.
//
// Without DATABASE_URL the test skips on your machine and fails in CI,
// where CI=true: a database test that silently skips in CI tests nothing.
func URL(t testing.TB) string {
	t.Helper()
	base := os.Getenv("DATABASE_URL")
	if base == "" {
		if os.Getenv("CI") == "true" {
			t.Fatal("DATABASE_URL is not set; CI must provide a Postgres database")
		}
		t.Skip("DATABASE_URL is not set; start Postgres and set it to run this test")
	}
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		t.Fatalf("DATABASE_URL must be a postgres:// URL, got %q", base)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatalf("connect to DATABASE_URL: %v", err)
	}
	defer conn.Close(ctx)

	random := make([]byte, 6)
	_, _ = rand.Read(random) // crypto/rand.Read never returns an error since Go 1.24
	schema := "test_" + hex.EncodeToString(random)
	if _, err := conn.Exec(ctx, "create schema "+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		conn, err := pgx.Connect(ctx, base)
		if err != nil {
			t.Errorf("drop schema %s: %v", schema, err)
			return
		}
		defer conn.Close(ctx)
		if _, err := conn.Exec(ctx, "drop schema "+schema+" cascade"); err != nil {
			t.Errorf("drop schema %s: %v", schema, err)
		}
	})

	// pgx sends query parameters it does not know, such as search_path,
	// to the server as settings for every connection.
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String()
}

// endregion: url
