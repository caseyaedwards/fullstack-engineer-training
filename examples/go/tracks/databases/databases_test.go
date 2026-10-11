package databases

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// testPool opens a pool on a new, empty schema in the database that
// DATABASE_URL names, creates the tables from Schema in it, and drops the
// schema when the test ends. It follows dbtest.URL from Week 8: without
// DATABASE_URL, the test skips on your machine and fails in CI, where
// CI=true.
func testPool(t *testing.T) *pgxpool.Pool {
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
	rand.Read(random)
	schema := "track_" + hex.EncodeToString(random)
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

	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, Schema); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	return pool
}

// addMonitor inserts a monitor that has never been checked and returns its id.
func addMonitor(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`insert into monitors (url) values ('https://example.com') returning id`).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// state reads what the two tables hold for one monitor: its last_status,
// nil while it has never been checked, and its number of checks.
func state(t *testing.T, pool *pgxpool.Pool, monitorID int64) (status *string, checks int) {
	t.Helper()
	ctx := context.Background()
	err := pool.QueryRow(ctx, `select last_status from monitors where id = $1`, monitorID).Scan(&status)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `select count(*) from checks where monitor_id = $1`, monitorID).Scan(&checks); err != nil {
		t.Fatal(err)
	}
	return status, checks
}

// region: tests

func TestRecordCheckWritesBothRows(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	id := addMonitor(t, pool)

	if err := RecordCheck(ctx, pool, id, false, 1840); err != nil {
		t.Fatalf("RecordCheck: %v", err)
	}
	status, checks := state(t, pool, id)
	if status == nil || *status != "down" || checks != 1 {
		t.Fatalf("after a failed check: last_status=%v checks=%d, want down and 1", deref(status), checks)
	}

	var ok bool
	var ms int
	err := pool.QueryRow(ctx, `select ok, ms from checks where monitor_id = $1`, id).Scan(&ok, &ms)
	if err != nil || ok || ms != 1840 {
		t.Fatalf("checks row: ok=%v ms=%d err=%v, want false 1840", ok, ms, err)
	}

	if err := RecordCheck(ctx, pool, id, true, 52); err != nil {
		t.Fatalf("RecordCheck: %v", err)
	}
	status, checks = state(t, pool, id)
	if status == nil || *status != "up" || checks != 2 {
		t.Fatalf("after a good check: last_status=%v checks=%d, want up and 2", deref(status), checks)
	}
}

func TestRecordCheckUnknownMonitorWritesNothing(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	err := RecordCheck(ctx, pool, 999, true, 52)
	if !errors.Is(err, ErrUnknownMonitor) {
		t.Fatalf("RecordCheck(999) = %v, want ErrUnknownMonitor", err)
	}
	var total int
	if err := pool.QueryRow(ctx, `select count(*) from checks`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 0 {
		t.Fatalf("checks holds %d rows after an unknown monitor, want 0", total)
	}
}

// The update of monitors succeeds, then the insert into checks breaks the
// check constraint ms >= 0. The rollback undoes the update too.
func TestRecordCheckRollsBackTheUpdate(t *testing.T) {
	pool := testPool(t)
	id := addMonitor(t, pool)

	if err := RecordCheck(context.Background(), pool, id, true, -1); err == nil {
		t.Fatal("RecordCheck with ms=-1 returned nil, want a check constraint error")
	}
	status, checks := state(t, pool, id)
	if status != nil || checks != 0 {
		t.Fatalf("after a failed insert: last_status=%v checks=%d, want NULL and 0", deref(status), checks)
	}
}

// endregion: tests

func deref(s *string) string {
	if s == nil {
		return "NULL"
	}
	return *s
}
