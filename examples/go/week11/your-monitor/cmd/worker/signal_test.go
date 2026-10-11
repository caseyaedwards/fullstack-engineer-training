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

	"fsp/examples/week11/your-monitor/internal/dbtest"
	"fsp/examples/week11/your-monitor/internal/queue"
)

// region: signals

// TestSignalReturnsJobsToTheQueue builds the worker, gives it three jobs
// for sites that never answer, and sends SIGTERM or SIGINT while all three
// checks wait. The worker must exit at once with code 0, and every job
// must still wait in the queue, untouched, for the next worker.
func TestSignalReturnsJobsToTheQueue(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "worker")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	for _, sig := range []os.Signal{syscall.SIGTERM, syscall.SIGINT} {
		t.Run(sig.String(), func(t *testing.T) {
			db, url := dbtest.NewWithURL(t)
			started := make(chan struct{}, 3)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				started <- struct{}{}
				<-r.Context().Done() // never answer; return once the client gives up
			}))
			t.Cleanup(srv.Close)
			for i := range 3 {
				dbtest.Exec(t, db, `INSERT INTO monitors (url, interval_seconds) VALUES ($1, 60)`, fmt.Sprintf("%s/%d", srv.URL, i))
			}
			if _, err := queue.Enqueue(context.Background(), db); err != nil {
				t.Fatal(err)
			}

			// CommandContext sends SIGKILL when ctx runs out: a hung worker fails the test.
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, bin, "-workers", "3", "-timeout", "1m")
			cmd.Env = append(os.Environ(), "DATABASE_URL="+url)
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
			if err := cmd.Process.Signal(sig); err != nil {
				t.Fatal(err)
			}
			var lines []string
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				lines = append(lines, scanner.Text())
			}
			cmd.Wait()

			if code := cmd.ProcessState.ExitCode(); code != 0 {
				t.Errorf("exit code = %d, want 0\n%s", code, strings.Join(lines, "\n"))
			}
			if d := time.Since(sent); d > 5*time.Second {
				t.Errorf("the worker took %v to exit after %v; the checks in flight were not cancelled", d, sig)
			}
			if !slices.ContainsFunc(lines, func(l string) bool { return strings.HasSuffix(l, " stopped") }) {
				t.Errorf("no stopped line in the log:\n%s", strings.Join(lines, "\n"))
			}
			if n := dbtest.Count(t, db, `SELECT count(*) FROM check_jobs WHERE attempts = 0`); n != 3 {
				t.Errorf("%d untouched jobs in the queue after %v, want 3", n, sig)
			}
			if n := dbtest.Count(t, db, `SELECT count(*) FROM check_results`); n != 0 {
				t.Errorf("%d results stored, want 0: a cancelled check is not a result", n)
			}
		})
	}
}

// endregion: signals
