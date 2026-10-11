package main

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"fsp/examples/week14/your-monitor/internal/db"
	"fsp/examples/week14/your-monitor/internal/db/dbtest"
	"fsp/examples/week14/your-monitor/internal/monitor"
	"fsp/examples/week14/your-monitor/internal/queue"
)

// build compiles the worker into a temporary directory.
func build(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "worker")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// region: signals

// TestSignalReturnsJobsToTheQueue is the Week 8 signal test for a worker
// that takes jobs from the queue. It gives the worker three jobs for sites
// that never answer, and sends a signal while all three checks wait. On
// SIGTERM and SIGINT the worker exits at once with code 0; SIGKILL ends it
// with no chance to clean up. Either way, every job still waits in the
// queue, untouched, and a second worker claims all three: Postgres rolls
// back the transaction of a connection that closes, and frees its locks.
func TestSignalReturnsJobsToTheQueue(t *testing.T) {
	bin := build(t)
	tests := []struct {
		sig         os.Signal
		wantCode    int // -1: the signal ended the process
		wantStopped bool
	}{
		{syscall.SIGTERM, 0, true},
		{syscall.SIGINT, 0, true},
		{syscall.SIGKILL, -1, false},
	}
	for _, tt := range tests {
		t.Run(tt.sig.String(), func(t *testing.T) {
			url := dbtest.URL(t)
			pool := dbtest.Pool(t, url, db.Migrate)
			started := make(chan struct{}, 3)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				started <- struct{}{}
				<-r.Context().Done() // never answer; return once the client gives up
			}))
			t.Cleanup(srv.Close)
			for i := range 3 {
				dbtest.Exec(t, pool, `INSERT INTO monitors (url, interval_seconds) VALUES ($1, 60)`, fmt.Sprintf("%s/%d", srv.URL, i))
			}
			if _, err := queue.Enqueue(context.Background(), pool); err != nil {
				t.Fatal(err)
			}

			// CommandContext sends SIGKILL when ctx runs out: a hung worker fails the test.
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, bin, "-database-url", url, "-workers", "3", "-timeout", "1m")
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			for range 3 { // wait until all three checks wait on the site
				select {
				case <-started:
				case <-time.After(10 * time.Second):
					t.Fatal("the worker did not start three checks within 10 s")
				}
			}
			sent := time.Now()
			if err := cmd.Process.Signal(tt.sig); err != nil {
				t.Fatal(err)
			}
			var lines []string
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				lines = append(lines, scanner.Text())
			}
			cmd.Wait()

			if code := cmd.ProcessState.ExitCode(); code != tt.wantCode {
				t.Errorf("exit code = %d (%v), want %d\n%s", code, cmd.ProcessState, tt.wantCode, strings.Join(lines, "\n"))
			}
			if d := time.Since(sent); d > 5*time.Second {
				t.Errorf("the worker took %v to exit after %v; the checks in flight were not cancelled", d, tt.sig)
			}
			stopped := slices.ContainsFunc(lines, func(l string) bool { return strings.Contains(l, `"msg":"stopped"`) })
			if stopped != tt.wantStopped {
				t.Errorf("logged stopped = %v, want %v\n%s", stopped, tt.wantStopped, strings.Join(lines, "\n"))
			}
			if n := dbtest.Count(t, pool, `SELECT count(*) FROM check_jobs WHERE attempts = 0`); n != 3 {
				t.Errorf("%d untouched jobs in the queue after %v, want 3", n, tt.sig)
			}
			if n := dbtest.Count(t, pool, `SELECT count(*) FROM check_results`); n != 0 {
				t.Errorf("%d results stored, want 0: a cancelled check is not a result", n)
			}
			if n := secondWorker(t, pool); n != 3 {
				t.Errorf("a second worker claimed %d jobs after %v, want 3", n, tt.sig)
			}
		})
	}
}

// endregion: signals

// secondWorker claims jobs with a stub check, the way the next worker
// does, and returns how many it claimed. Postgres frees the locks of a
// killed worker once it reads the end of the connection, a moment after
// the process dies: secondWorker tries for up to 5 seconds.
func secondWorker(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	stub := func(_ context.Context, job queue.Job) monitor.Result {
		return monitor.Result{MonitorID: job.MonitorID, CheckedAt: time.Now(), Up: true}
	}
	claimed := 0
	for deadline := time.Now().Add(5 * time.Second); claimed < 3 && time.Now().Before(deadline); {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		worked, err := queue.Work(ctx, pool, stub)
		cancel()
		if err != nil {
			t.Fatalf("second worker: %v", err)
		}
		if worked {
			claimed++
		} else {
			time.Sleep(20 * time.Millisecond)
		}
	}
	return claimed
}
