package queue

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"fsp/examples/week14/your-monitor/internal/db"
	"fsp/examples/week14/your-monitor/internal/db/dbtest"
	"fsp/examples/week14/your-monitor/internal/monitor"
)

// newDB returns a pool on an empty schema with every migration applied.
func newDB(t *testing.T) *pgxpool.Pool {
	return dbtest.Pool(t, dbtest.URL(t), db.Migrate)
}

// addMonitors inserts n monitors with a 60-second interval.
func addMonitors(t *testing.T, pool *pgxpool.Pool, n int) {
	t.Helper()
	dbtest.Exec(t, pool, `
		INSERT INTO monitors (url, interval_seconds)
		SELECT 'https://site' || i || '.example', 60 FROM generate_series(1, $1) AS i`, n)
}

// waitBlocked waits until n backends wait for a lock that the backend
// with process ID pid holds. It proves that the goroutines of a test
// overlap, instead of hoping that they do.
func waitBlocked(t *testing.T, pool *pgxpool.Pool, pid uint32, n int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	// A backend that waits behind another waiter counts too.
	const blockedSQL = `
		WITH RECURSIVE blocked AS (
		    SELECT pid FROM pg_stat_activity WHERE $1 = ANY(pg_blocking_pids(pid))
		    UNION
		    SELECT a.pid FROM pg_stat_activity a JOIN blocked b ON b.pid = ANY(pg_blocking_pids(a.pid))
		)
		SELECT count(*) FROM blocked`
	for dbtest.Count(t, pool, blockedSQL, pid) < n {
		if time.Now().After(deadline) {
			t.Fatalf("fewer than %d backends wait for the lock of backend %d after 10 s", n, pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// up returns a CheckFunc result for a site that answered 200.
func up(job Job) monitor.Result {
	code := 200
	return monitor.Result{MonitorID: job.MonitorID, CheckedAt: time.Now(), Up: true, StatusCode: &code}
}

// region: enqueue-test

func TestEnqueueAddsOneJobPerDueMonitor(t *testing.T) {
	pool := newDB(t)
	ctx := context.Background()
	addMonitors(t, pool, 3)

	steps := []struct {
		name  string
		setup string // SQL to run before Enqueue
		want  int64
	}{
		{"never queued: all three are due", "", 3},
		{"jobs still waiting: none is due", "", 0},
		{"jobs still waiting, queued 60 s ago: NOT EXISTS keeps them from piling up",
			"UPDATE monitors SET last_enqueued_at = now() - interval '60 seconds'", 0},
		{"jobs done, queued 10 s ago: not due yet",
			"DELETE FROM check_jobs; UPDATE monitors SET last_enqueued_at = now() - interval '10 seconds'", 0},
		{"queued 60 s ago: due again",
			"UPDATE monitors SET last_enqueued_at = now() - interval '60 seconds'", 3},
	}
	for _, s := range steps {
		if s.setup != "" {
			dbtest.Exec(t, pool, s.setup)
		}
		got, err := Enqueue(ctx, pool)
		if err != nil {
			t.Fatal(err)
		}
		if got != s.want {
			t.Errorf("%s: Enqueue added %d jobs, want %d", s.name, got, s.want)
		}
	}
}

// TestTwoSchedulersDoNotQueueAMonitorTwice starts four schedulers at the
// same moment. A test transaction locks every monitor first, and the four
// Enqueue calls wait for it, all inside their UPDATE. Then it commits, and
// all four go at once.
func TestTwoSchedulersDoNotQueueAMonitorTwice(t *testing.T) {
	pool := newDB(t)
	ctx := context.Background()
	addMonitors(t, pool, 50)

	gate, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Rollback(ctx)
	if _, err := gate.Exec(ctx, `SELECT id FROM monitors FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	var gatePID uint32
	if err := gate.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&gatePID); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	var total atomic.Int64
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, err := Enqueue(context.Background(), pool)
			if err != nil {
				t.Error(err)
			}
			total.Add(n)
		}()
	}
	waitBlocked(t, pool, gatePID, 4)
	if err := gate.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if got := total.Load(); got != 50 {
		t.Errorf("four schedulers added %d jobs in total, want 50", got)
	}
	if n := dbtest.Count(t, pool, `SELECT count(DISTINCT monitor_id) FROM check_jobs`); n != 50 {
		t.Errorf("%d monitors have a job, want 50", n)
	}
}

// endregion: enqueue-test

// region: once-test

// TestEachJobRunsOnce starts four workers on 200 jobs. Every job must run
// exactly once, and the workers must overlap, or the test proves nothing.
func TestEachJobRunsOnce(t *testing.T) {
	pool := newDB(t)
	ctx := context.Background()
	addMonitors(t, pool, 200)
	if _, err := Enqueue(ctx, pool); err != nil {
		t.Fatal(err)
	}

	var (
		mu               sync.Mutex
		runs             = map[int64]int{} // job ID to how often it ran
		inFlight, maxPar atomic.Int64
	)
	check := func(ctx context.Context, job Job) monitor.Result {
		n := inFlight.Add(1)
		defer inFlight.Add(-1)
		for old := maxPar.Load(); n > old && !maxPar.CompareAndSwap(old, n); old = maxPar.Load() {
		}
		mu.Lock()
		runs[job.ID]++
		mu.Unlock()
		time.Sleep(2 * time.Millisecond) // a check takes time: let the workers overlap
		return up(job)
	}

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				worked, err := Work(ctx, pool, check)
				if err != nil {
					t.Error(err)
					return
				}
				if !worked {
					return // the queue is empty
				}
			}
		}()
	}
	wg.Wait()

	if len(runs) != 200 {
		t.Errorf("%d different jobs ran, want 200", len(runs))
	}
	for id, n := range runs {
		if n != 1 {
			t.Errorf("job %d ran %d times, want 1", id, n)
		}
	}
	if got := maxPar.Load(); got < 2 {
		t.Errorf("at most %d check ran at a time, want the workers to overlap", got)
	}
	if n := dbtest.Count(t, pool, `SELECT count(*) FROM check_results`); n != 200 {
		t.Errorf("check_results has %d rows, want 200", n)
	}
	if n := dbtest.Count(t, pool, `SELECT count(*) FROM check_jobs`); n != 0 {
		t.Errorf("check_jobs has %d rows left, want 0", n)
	}
}

// endregion: once-test

// region: skip-test

// TestLockedJobIsSkipped holds the only job in one worker while a second
// worker looks for work. The second worker must find none, at once,
// instead of waiting for the lock.
func TestLockedJobIsSkipped(t *testing.T) {
	pool := newDB(t)
	ctx := context.Background()
	addMonitors(t, pool, 1)
	if _, err := Enqueue(ctx, pool); err != nil {
		t.Fatal(err)
	}

	claimed, release := make(chan struct{}), make(chan struct{})
	done := make(chan error)
	go func() {
		_, err := Work(ctx, pool, func(ctx context.Context, job Job) monitor.Result {
			close(claimed)
			<-release
			return up(job)
		})
		done <- err
	}()
	<-claimed

	// Without SKIP LOCKED, the second worker waits for the lock until the
	// first commits, which never happens here: the deadline ends the wait.
	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	worked, err := Work(waitCtx, pool, func(context.Context, Job) monitor.Result {
		t.Error("the second worker ran the job the first one holds")
		return monitor.Result{}
	})
	if err != nil || worked {
		t.Errorf("second Work = %v, %v; want false, nil: it must skip the locked job", worked, err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// endregion: skip-test

// region: crash-test

// TestCancelledWorkLeavesTheJob cancels a worker in the middle of a check,
// as SIGTERM does. The transaction rolls back and the job waits for the
// next worker, unchanged.
func TestCancelledWorkLeavesTheJob(t *testing.T) {
	pool := newDB(t)
	addMonitors(t, pool, 1)
	if _, err := Enqueue(context.Background(), pool); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	_, err := Work(ctx, pool, func(ctx context.Context, job Job) monitor.Result {
		cancel()
		msg := ctx.Err().Error()
		return monitor.Result{MonitorID: job.MonitorID, CheckedAt: time.Now(), Error: &msg}
	})
	if err == nil {
		t.Fatal("Work returned no error after its context was cancelled")
	}
	if n := dbtest.Count(t, pool, `SELECT count(*) FROM check_jobs WHERE attempts = 0`); n != 1 {
		t.Errorf("%d untouched jobs in the queue, want 1", n)
	}
	if n := dbtest.Count(t, pool, `SELECT count(*) FROM check_results`); n != 0 {
		t.Errorf("check_results has %d rows, want 0: a cancelled check is not a result", n)
	}

	worked, err := Work(context.Background(), pool, func(_ context.Context, job Job) monitor.Result { return up(job) })
	if !worked || err != nil {
		t.Errorf("next Work = %v, %v; want true, nil", worked, err)
	}
}

// endregion: crash-test

// TestCancelledWorkFreesTheJobAtOnce checks how Work rolls back on
// shutdown. A rollback with the cancelled context fails, and pgx then
// closes the connection: Postgres frees the job only once it notices, a
// moment later. Work rolls back with a context of its own, which frees
// the job at once and keeps the connection in the pool.
func TestCancelledWorkFreesTheJobAtOnce(t *testing.T) {
	pool := newDB(t)
	addMonitors(t, pool, 1)
	if _, err := Enqueue(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
	dbtest.Count(t, pool, `SELECT 1`) // one idle connection waits in the pool
	opened := pool.Stat().NewConnsCount()

	ctx, cancel := context.WithCancel(context.Background())
	Work(ctx, pool, func(_ context.Context, job Job) monitor.Result {
		cancel()
		return up(job)
	})
	// The next query reuses the connection of Work, unless Work closed it.
	dbtest.Count(t, pool, `SELECT 1`)
	if got := pool.Stat().NewConnsCount() - opened; got != 0 {
		t.Errorf("the pool opened %d more connection(s) after the cancelled Work, want 0: the rollback closed its connection", got)
	}
	// NOWAIT fails at once if any transaction still holds the job.
	dbtest.Exec(t, pool, `SELECT id FROM check_jobs FOR UPDATE NOWAIT`)
}

// region: delete-test

// TestDeleteDuringCheck deletes a monitor while a worker checks it. The
// DELETE must wait for the check and then remove the monitor, its job and
// the new result, with no deadlock on either side.
func TestDeleteDuringCheck(t *testing.T) {
	pool := newDB(t)
	ctx := context.Background()
	addMonitors(t, pool, 1)
	if _, err := Enqueue(ctx, pool); err != nil {
		t.Fatal(err)
	}

	// Buffered channels and a deferred release: when the test fails early,
	// the worker still finishes, and the pool closes instead of hanging.
	type backend struct {
		pid uint32
		err error
	}
	claimed, worked, deleted := make(chan backend, 1), make(chan error, 1), make(chan error, 1)
	release := make(chan struct{})
	var once sync.Once
	releaseCheck := func() { once.Do(func() { close(release) }) }
	defer releaseCheck()

	go func() {
		_, err := Work(ctx, pool, func(ctx context.Context, job Job) monitor.Result {
			var b backend // the backend of this worker's transaction
			b.err = pool.QueryRow(ctx, `SELECT pid FROM pg_locks WHERE locktype = 'relation'
				AND relation = 'check_jobs'::regclass AND mode = 'RowShareLock' LIMIT 1`).Scan(&b.pid)
			claimed <- b
			<-release
			return up(job)
		})
		worked <- err
	}()
	b := <-claimed
	if b.err != nil {
		t.Fatalf("find the backend that holds the job: %v (does the claim lock its row?)", b.err)
	}

	go func() {
		_, err := pool.Exec(ctx, `DELETE FROM monitors`)
		deleted <- err
	}()
	waitBlocked(t, pool, b.pid, 1) // the DELETE waits for the check
	releaseCheck()

	if err := <-worked; err != nil {
		t.Errorf("Work: %v", err)
	}
	if err := <-deleted; err != nil {
		t.Errorf("DELETE: %v", err)
	}
	for _, table := range []string{"monitors", "check_jobs", "check_results"} {
		if n := dbtest.Count(t, pool, `SELECT count(*) FROM `+table); n != 0 {
			t.Errorf("%s has %d rows after the DELETE, want 0", table, n)
		}
	}
}

// endregion: delete-test

// region: retry-test

// TestFailingJobIsRetriedThenDropped panics in every check. Each failure
// adds an attempt and moves run_at out; the third failure drops the job.
func TestFailingJobIsRetriedThenDropped(t *testing.T) {
	pool := newDB(t)
	ctx := context.Background()
	addMonitors(t, pool, 1)
	if _, err := Enqueue(ctx, pool); err != nil {
		t.Fatal(err)
	}
	boom := func(context.Context, Job) monitor.Result { panic("boom") }

	for attempt := 1; attempt <= MaxAttempts; attempt++ {
		worked, err := Work(ctx, pool, boom)
		if !worked || err == nil || !strings.Contains(err.Error(), "check panicked: boom") {
			t.Fatalf("attempt %d: Work = %v, %v; want true and the panic as an error", attempt, worked, err)
		}
		if attempt == MaxAttempts {
			if !strings.Contains(err.Error(), fmt.Sprintf("dropped after %d attempts", MaxAttempts)) {
				t.Errorf("last attempt: error %q does not say the job was dropped", err)
			}
			break
		}
		// The job waits 30 s per attempt before a worker sees it again.
		if worked, _ := Work(ctx, pool, boom); worked {
			t.Fatalf("attempt %d: a worker took the job before its new run_at", attempt)
		}
		dbtest.Exec(t, pool, `UPDATE check_jobs SET run_at = now()`)
	}
	if n := dbtest.Count(t, pool, `SELECT count(*) FROM check_jobs`); n != 0 {
		t.Errorf("check_jobs has %d rows, want 0 after %d failures", n, MaxAttempts)
	}
}

// endregion: retry-test

func TestDepthReportsTheOldestJob(t *testing.T) {
	pool := newDB(t)
	ctx := context.Background()
	addMonitors(t, pool, 2)
	if _, err := Enqueue(ctx, pool); err != nil {
		t.Fatal(err)
	}
	dbtest.Exec(t, pool, `UPDATE check_jobs SET run_at = now() - interval '90 seconds' WHERE id = (SELECT min(id) FROM check_jobs)`)
	queued, oldest, err := Depth(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if queued != 2 || oldest < 90*time.Second || oldest > 95*time.Second {
		t.Errorf("Depth = %d, %v; want 2 and about 90s", queued, oldest)
	}

	// Only retries wait, with run_at in the future: the age is 0, not negative.
	dbtest.Exec(t, pool, `UPDATE check_jobs SET run_at = now() + interval '30 seconds'`)
	if queued, oldest, err = Depth(ctx, pool); err != nil || queued != 2 || oldest != 0 {
		t.Errorf("Depth = %d, %v, %v; want 2, 0s, nil while only retries wait", queued, oldest, err)
	}
}
