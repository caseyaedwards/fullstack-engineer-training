package main

import (
	"bufio"
	"context"
	"fmt"
	"maps"
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

	"fsp/examples/week08/your-monitor/internal/db"
	"fsp/examples/week08/your-monitor/internal/db/dbtest"
	"fsp/examples/week08/your-monitor/internal/monitor"
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

// addMonitors creates one monitor per URL in the database at url, which
// also creates the tables.
func addMonitors(t *testing.T, url string, targets ...string) {
	t.Helper()
	ctx := context.Background()
	pool, err := db.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	store := db.NewMonitorStore(pool)
	for _, target := range targets {
		if _, err := store.Create(ctx, monitor.Monitor{URL: target, IntervalSeconds: 30}); err != nil {
			t.Fatal(err)
		}
	}
}

// TestProgramSignals is the Week 3 test, with its URL in the database
// instead of urls.txt. It runs the worker against a test server and sends
// it a signal once it has logged two checks.
func TestProgramSignals(t *testing.T) {
	bin := build(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(srv.Close)
	url := dbtest.URL(t)
	addMonitors(t, url, srv.URL)

	tests := []struct {
		name        string
		sig         os.Signal
		wantCode    int // -1: the signal ended the process
		wantStopped bool
	}{
		{"SIGTERM shuts down cleanly", syscall.SIGTERM, 0, true},
		{"SIGINT shuts down cleanly", syscall.SIGINT, 0, true},
		{"SIGKILL ends it at once", syscall.SIGKILL, -1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// CommandContext sends SIGKILL when ctx runs out: a hung worker fails the test.
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, bin, "-database-url", url, "-interval", "50ms")
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}

			var lines []string
			scanner := bufio.NewScanner(stdout)
			for checks := 0; checks < 2 && scanner.Scan(); {
				lines = append(lines, scanner.Text())
				if strings.Contains(scanner.Text(), "url="+srv.URL) {
					checks++
				}
			}
			if err := cmd.Process.Signal(tt.sig); err != nil {
				t.Fatal(err)
			}
			for scanner.Scan() { // read to the end: the process has closed stdout
				lines = append(lines, scanner.Text())
			}
			cmd.Wait()

			if code := cmd.ProcessState.ExitCode(); code != tt.wantCode {
				t.Errorf("exit code = %d (%v), want %d", code, cmd.ProcessState, tt.wantCode)
			}
			stopped := slices.ContainsFunc(lines, func(l string) bool { return strings.HasSuffix(l, " stopped") })
			if stopped != tt.wantStopped {
				t.Errorf("logged stopped = %v, want %v\n%s", stopped, tt.wantStopped, strings.Join(lines, "\n"))
			}
		})
	}
}

// region: signals

// TestWorkerStoresResults runs the worker against two sites, one up and
// one that returns 500. Every finished check must be a row in
// check_results. Then SIGTERM arrives while three
// checks wait on a site that never answers: those three are cancelled
// and leave no rows.
func TestWorkerStoresResults(t *testing.T) {
	bin := build(t)
	started := make(chan struct{}, 10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fail":
			w.WriteHeader(http.StatusInternalServerError)
		case "/hang":
			started <- struct{}{}
			<-r.Context().Done() // never answer; return once the client gives up
		}
	}))
	t.Cleanup(srv.Close)
	url := dbtest.URL(t)
	addMonitors(t, url, srv.URL+"/ok", srv.URL+"/fail")

	ctx := context.Background()
	pool, err := db.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, bin, "-database-url", url, "-interval", "200ms", "-workers", "3", "-timeout", "1m")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var lines []string
	scanner := bufio.NewScanner(stdout)
	for rounds := 0; rounds < 2 && scanner.Scan(); {
		lines = append(lines, scanner.Text())
		if strings.Contains(scanner.Text(), " round checks=") {
			rounds++
		}
	}
	// The worker reads the monitors table each round: three new monitors
	// that never answer join the next round.
	addMonitors(t, url, srv.URL+"/hang?1", srv.URL+"/hang?2", srv.URL+"/hang?3")
	for range 3 { // wait until all three workers wait on a site
		select {
		case <-started:
		case <-time.After(10 * time.Second):
			t.Fatal("the worker did not start three checks of the new monitors within 10 s")
		}
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	cmd.Wait()
	log := strings.Join(lines, "\n")
	if code := cmd.ProcessState.ExitCode(); code != 0 {
		t.Errorf("exit code = %d, want 0\n%s", code, log)
	}
	if !strings.Contains(log, "checks=5 up=1 down=1 timeouts=0 cancelled=3 skipped=0") {
		t.Errorf("want a last round with 1 up, 1 down and 3 cancelled:\n%s", log)
	}

	// One row per round for each answering site; nothing for /hang.
	rounds := strings.Count(log, " round checks=")
	rows, err := pool.Query(ctx, `select m.url, r.up, r.status_code
		from check_results r join monitors m on m.id = r.monitor_id`)
	if err != nil {
		t.Fatal(err)
	}
	count := map[string]int{}
	for rows.Next() {
		var u string
		var up bool
		var status *int
		if err := rows.Scan(&u, &up, &status); err != nil {
			t.Fatal(err)
		}
		count[fmt.Sprintf("%s up=%v status=%d", strings.TrimPrefix(u, srv.URL), up, *status)]++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"/ok up=true status=200": rounds, "/fail up=false status=500": rounds}
	if !maps.Equal(count, want) {
		t.Errorf("check_results rows = %v, want %v", count, want)
	}
}

// endregion: signals
