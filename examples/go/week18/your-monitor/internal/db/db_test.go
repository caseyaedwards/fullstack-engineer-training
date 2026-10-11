package db

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"fsp/examples/week18/your-monitor/internal/db/dbtest"
	"fsp/examples/week18/your-monitor/internal/monitor"
)

// connect opens a pool on a fresh, empty schema.
func connect(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := Connect(context.Background(), dbtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// migrated opens a pool on a fresh schema with every migration applied.
func migrated(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := connect(t)
	if err := Migrate(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
	return pool
}

func versions(t *testing.T, pool *pgxpool.Pool) []int {
	t.Helper()
	rows, err := pool.Query(context.Background(), "select version from schema_migrations order by version")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var vs []int
	for rows.Next() {
		var v int
		rows.Scan(&v)
		vs = append(vs, v)
	}
	return vs
}

// region: migratetest

func TestMigrateRunsEachFileOnce(t *testing.T) {
	pool := connect(t)
	ctx := context.Background()
	// The api and the worker start together: five programs migrate at once.
	var wg sync.WaitGroup
	errs := make(chan error, 5)
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- Migrate(ctx, pool)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Migrate: %v", err)
		}
	}
	if err := Migrate(ctx, pool); err != nil { // and once more, later
		t.Fatal(err)
	}
	// Week 9 adds 002_jobs_alerts.sql, and Week 11 003_auth.sql.
	if got := versions(t, pool); !slices.Equal(got, []int{1, 2, 3}) {
		t.Errorf("schema_migrations versions = %v, want [1 2 3]", got)
	}
}

func TestFailedMigrationLeavesNoTrace(t *testing.T) {
	pool := connect(t)
	ctx := context.Background()
	files := fstest.MapFS{
		"001_ok.sql":     {Data: []byte("create table a (id int);")},
		"002_broken.sql": {Data: []byte("create table b (id int); insert into b values ('not a number');")},
	}
	err := migrate(ctx, pool, files)
	if err == nil || !strings.Contains(err.Error(), "002_broken.sql") {
		t.Fatalf("migrate = %v, want an error that names 002_broken.sql", err)
	}
	if got := versions(t, pool); len(got) != 1 || got[0] != 1 {
		t.Errorf("versions = %v, want [1]: 002 must not be recorded", got)
	}
	// Table b was created inside the failed transaction: it must be gone.
	var exists bool
	pool.QueryRow(ctx, "select to_regclass('b') is not null").Scan(&exists)
	if exists {
		t.Error("table b exists: the failed migration was not rolled back")
	}
}

// endregion: migratetest

// region: storetest

func TestMonitorStore(t *testing.T) {
	ctx := context.Background()
	store := NewMonitorStore(migrated(t))

	m, err := store.Create(ctx, monitor.Monitor{URL: "https://example.com", IntervalSeconds: 60})
	if err != nil {
		t.Fatal(err)
	}
	if m.ID == 0 || m.CreatedAt.IsZero() {
		t.Errorf("Create = %+v, want the ID and CreatedAt from Postgres", m)
	}
	if _, err := store.Create(ctx, monitor.Monitor{URL: "https://example.com", IntervalSeconds: 300}); !errors.Is(err, monitor.ErrDuplicate) {
		t.Errorf("second Create of the same URL = %v, want ErrDuplicate", err)
	}
	// The check constraint holds even for code that skips monitor.Validate.
	if _, err := store.Create(ctx, monitor.Monitor{URL: "https://example.org", IntervalSeconds: 5}); err == nil ||
		!strings.Contains(err.Error(), "monitors_interval_seconds_check") {
		t.Errorf("Create with a 5 s interval = %v, want a check constraint violation", err)
	}

	got, err := store.Get(ctx, m.ID)
	if err != nil || got.URL != m.URL || !got.CreatedAt.Equal(m.CreatedAt) {
		t.Errorf("Get = %+v, %v, want %+v", got, err, m)
	}
	list, err := store.List(ctx)
	if err != nil || len(list) != 1 {
		t.Errorf("List = %v, %v, want one monitor", list, err)
	}
	if err := store.Delete(ctx, m.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, m.ID); !errors.Is(err, monitor.ErrNotFound) {
		t.Errorf("Get after Delete = %v, want ErrNotFound", err)
	}
	if err := store.Delete(ctx, m.ID); !errors.Is(err, monitor.ErrNotFound) {
		t.Errorf("second Delete = %v, want ErrNotFound", err)
	}
	if list, _ := store.List(ctx); list == nil || len(list) != 0 {
		t.Errorf("List of an empty table = %#v, want an empty, non-nil slice", list)
	}
}

func TestResultStore(t *testing.T) {
	ctx := context.Background()
	pool := migrated(t)
	m, err := NewMonitorStore(pool).Create(ctx, monitor.Monitor{URL: "https://example.com", IntervalSeconds: 60})
	if err != nil {
		t.Fatal(err)
	}
	results := NewResultStore(pool)

	start := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	status, dns, msg := 200, 0, "boom"
	for i := range 5 {
		r := monitor.Result{MonitorID: m.ID, CheckedAt: start.Add(time.Duration(i) * time.Minute), Up: true, StatusCode: &status, DNSMs: &dns}
		if i == 4 {
			r = monitor.Result{MonitorID: m.ID, CheckedAt: start.Add(4 * time.Minute), Error: &msg}
		}
		if err := results.Insert(ctx, r); err != nil {
			t.Fatal(err)
		}
	}

	got, err := results.Recent(ctx, m.ID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("Recent(3) returned %d rows", len(got))
	}
	for i, r := range got {
		if want := start.Add(time.Duration(4-i) * time.Minute); !r.CheckedAt.Equal(want) {
			t.Errorf("row %d checked_at %v, want %v: newest first", i, r.CheckedAt, want)
		}
	}
	if got[0].Up || got[0].StatusCode != nil || got[0].DNSMs != nil || *got[0].Error != "boom" {
		t.Errorf("failed check = %+v, want up false, NULL status and dns_ms, error boom", got[0])
	}
	if got[1].DNSMs == nil || *got[1].DNSMs != 0 {
		t.Errorf("dns_ms = %v, want 0, not NULL", got[1].DNSMs)
	}

	if none, err := results.Recent(ctx, m.ID+1, 50); err != nil || none == nil || len(none) != 0 {
		t.Errorf("Recent of an unknown monitor = %#v, %v, want an empty, non-nil slice", none, err)
	}
	// The foreign key refuses a result for a monitor that does not exist.
	if err := results.Insert(ctx, monitor.Result{MonitorID: m.ID + 1, CheckedAt: start}); err == nil ||
		!strings.Contains(err.Error(), "check_results_monitor_id_fkey") {
		t.Errorf("Insert for a missing monitor = %v, want a foreign key violation", err)
	}
}

// endregion: storetest

// region: indextest

// TestRecentUsesTheIndex fills check_results with a day of checks for 20
// monitors, then asks Postgres for its plan of the Recent query. With the
// index, Postgres reads the newest rows of one monitor and stops; without
// it, it reads all 57,600 rows and sorts them.
func TestRecentUsesTheIndex(t *testing.T) {
	ctx := context.Background()
	pool := migrated(t)
	_, err := pool.Exec(ctx, `
		insert into monitors (url, interval_seconds)
		select 'https://site' || n || '.example', 30 from generate_series(1, 20) n;
		insert into check_results (monitor_id, checked_at, up)
		select m.id, now() - n * interval '30 seconds', true
		  from monitors m, generate_series(1, 2880) n;
		analyze check_results;`)
	if err != nil {
		t.Fatal(err)
	}
	plan := explain(t, pool)
	if !strings.Contains(plan, "Index Scan using check_results_monitor_time") {
		t.Errorf("plan does not use the index:\n%s", plan)
	}

	if _, err := pool.Exec(ctx, "drop index check_results_monitor_time"); err != nil {
		t.Fatal(err)
	}
	if plan := explain(t, pool); !strings.Contains(plan, "Seq Scan on check_results") {
		t.Errorf("without the index, want a Seq Scan:\n%s", plan)
	}
}

func explain(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	rows, err := pool.Query(context.Background(), "explain "+recentSQL, 7, 50)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var lines []string
	for rows.Next() {
		var line string
		rows.Scan(&line)
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// endregion: indextest
