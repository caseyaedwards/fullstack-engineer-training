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

	"fsp/examples/week13/your-monitor/internal/dbtest"
	"fsp/examples/week13/your-monitor/internal/monitor"
)

// addMonitors inserts n monitors with a 60-second interval.
func addMonitors(t *testing.T, db *pgxpool.Pool, n int) {
	t.Helper()
	dbtest.Exec(t, db, `
		INSERT INTO monitors (url, interval_seconds)
		SELECT 'https://site' || i || '.example', 60 FROM generate_series(1, $1) AS i`, n)
}

// up returns a CheckFunc result for a site that answered 200.
func up(job Job) monitor.Result {
	return monitor.Result{MonitorID: job.MonitorID, CheckedAt: time.Now(), Up: true, StatusCode: 200}
}

// region: enqueue-test

func TestEnqueueAddsOneJobPerDueMonitor(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	addMonitors(t, db, 3)

	steps := []struct {
		name  string
		setup string // SQL to run before Enqueue
		want  int64
	}{
		{"never queued: all three are due", "", 3},
		{"jobs still waiting: none is due", "", 0},
		{"jobs done, queued 10 s ago: not due yet",
			"DELETE FROM check_jobs; UPDATE monitors SET last_enqueued_at = now() - interval '10 seconds'", 0},
		{"queued 60 s ago: due again",
			"UPDATE monitors SET last_enqueued_at = now() - interval '60 seconds'", 3},
	}
	for _, s := range steps {
		if s.setup != "" {
			dbtest.Exec(t, db, s.setup)
		}
		got, err := Enqueue(ctx, db)
		if err != nil {
			t.Fatal(err)
		}
		if got != s.want {
			t.Errorf("%s: Enqueue added %d jobs, want %d", s.name, got, s.want)
		}
	}
}

func TestTwoSchedulersDoNotQueueAMonitorTwice(t *testing.T) {
	db := dbtest.New(t)
	addMonitors(t, db, 50)

	var wg sync.WaitGroup
	var total atomic.Int64
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, err := Enqueue(context.Background(), db)
			if err != nil {
				t.Error(err)
			}
			total.Add(n)
		}()
	}
	wg.Wait()
	if got := total.Load(); got != 50 {
		t.Errorf("four schedulers added %d jobs in total, want 50", got)
	}
	if n := dbtest.Count(t, db, `SELECT count(DISTINCT monitor_id) FROM check_jobs`); n != 50 {
		t.Errorf("%d monitors have a job, want 50", n)
	}
}

// endregion: enqueue-test

// region: once-test

// TestEachJobRunsOnce starts four workers on 200 jobs. Every job must run
// exactly once, and the workers must overlap, or the test proves nothing.
func TestEachJobRunsOnce(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	addMonitors(t, db, 200)
	if _, err := Enqueue(ctx, db); err != nil {
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
				worked, err := Work(ctx, db, check)
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
	if n := dbtest.Count(t, db, `SELECT count(*) FROM check_results`); n != 200 {
		t.Errorf("check_results has %d rows, want 200", n)
	}
	if n := dbtest.Count(t, db, `SELECT count(*) FROM check_jobs`); n != 0 {
		t.Errorf("check_jobs has %d rows left, want 0", n)
	}
}

// endregion: once-test

// region: skip-test

// TestLockedJobIsSkipped holds the only job in one worker while a second
// worker looks for work. The second worker must find none, at once,
// instead of waiting for the lock.
func TestLockedJobIsSkipped(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	addMonitors(t, db, 1)
	if _, err := Enqueue(ctx, db); err != nil {
		t.Fatal(err)
	}

	claimed, release := make(chan struct{}), make(chan struct{})
	done := make(chan error)
	go func() {
		_, err := Work(ctx, db, func(ctx context.Context, job Job) monitor.Result {
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
	worked, err := Work(waitCtx, db, func(context.Context, Job) monitor.Result {
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
	db := dbtest.New(t)
	addMonitors(t, db, 1)
	if _, err := Enqueue(context.Background(), db); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	_, err := Work(ctx, db, func(ctx context.Context, job Job) monitor.Result {
		cancel()
		return monitor.Result{MonitorID: job.MonitorID, CheckedAt: time.Now(), Error: ctx.Err().Error()}
	})
	if err == nil {
		t.Fatal("Work returned no error after its context was cancelled")
	}
	if n := dbtest.Count(t, db, `SELECT count(*) FROM check_jobs WHERE attempts = 0`); n != 1 {
		t.Errorf("%d untouched jobs in the queue, want 1", n)
	}
	if n := dbtest.Count(t, db, `SELECT count(*) FROM check_results`); n != 0 {
		t.Errorf("check_results has %d rows, want 0: a cancelled check is not a result", n)
	}

	worked, err := Work(context.Background(), db, func(_ context.Context, job Job) monitor.Result { return up(job) })
	if !worked || err != nil {
		t.Errorf("next Work = %v, %v; want true, nil", worked, err)
	}
}

// endregion: crash-test

// region: retry-test

// TestFailingJobIsRetriedThenDropped panics in every check. Each failure
// adds an attempt and moves run_at out; the third failure drops the job.
func TestFailingJobIsRetriedThenDropped(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	addMonitors(t, db, 1)
	if _, err := Enqueue(ctx, db); err != nil {
		t.Fatal(err)
	}
	boom := func(context.Context, Job) monitor.Result { panic("boom") }

	for attempt := 1; attempt <= MaxAttempts; attempt++ {
		worked, err := Work(ctx, db, boom)
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
		if worked, _ := Work(ctx, db, boom); worked {
			t.Fatalf("attempt %d: a worker took the job before its new run_at", attempt)
		}
		dbtest.Exec(t, db, `UPDATE check_jobs SET run_at = now()`)
	}
	if n := dbtest.Count(t, db, `SELECT count(*) FROM check_jobs`); n != 0 {
		t.Errorf("check_jobs has %d rows, want 0 after %d failures", n, MaxAttempts)
	}
}

// endregion: retry-test

func TestDepthReportsTheOldestJob(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	addMonitors(t, db, 2)
	if _, err := Enqueue(ctx, db); err != nil {
		t.Fatal(err)
	}
	dbtest.Exec(t, db, `UPDATE check_jobs SET run_at = now() - interval '90 seconds' WHERE id = (SELECT min(id) FROM check_jobs)`)
	queued, oldest, err := Depth(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if queued != 2 || oldest < 90*time.Second || oldest > 95*time.Second {
		t.Errorf("Depth = %d, %v; want 2 and about 90s", queued, oldest)
	}
}
