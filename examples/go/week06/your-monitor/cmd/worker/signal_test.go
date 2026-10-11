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
)

// TestProgramSignals is the Week 3 test, unchanged. It builds the monitor,
// runs it against a test server, and sends it a signal once it has logged
// two checks.
func TestProgramSignals(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "your-monitor")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(srv.Close)
	file := filepath.Join(t.TempDir(), "urls.txt")
	if err := os.WriteFile(file, []byte(srv.URL+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

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
			// CommandContext sends SIGKILL when ctx runs out: a hung monitor fails the test.
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, bin, "-file", file, "-interval", "50ms")
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

// TestSignalCancelsChecksInFlight builds the monitor, points it at six URLs
// that never answer, and sends SIGTERM or SIGINT while three workers wait
// on them. The checks in flight must stop at once, not after -timeout.
func TestSignalCancelsChecksInFlight(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "your-monitor")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	for _, sig := range []os.Signal{syscall.SIGTERM, syscall.SIGINT} {
		t.Run(sig.String(), func(t *testing.T) {
			started := make(chan struct{}, 6)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				started <- struct{}{}
				<-r.Context().Done() // never answer; return once the client gives up
			}))
			t.Cleanup(srv.Close)
			var urls strings.Builder
			for i := range 6 {
				fmt.Fprintf(&urls, "%s/%d\n", srv.URL, i)
			}
			file := filepath.Join(t.TempDir(), "urls.txt")
			if err := os.WriteFile(file, []byte(urls.String()), 0o644); err != nil {
				t.Fatal(err)
			}

			// CommandContext sends SIGKILL when ctx runs out: a hung monitor fails the test.
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, bin, "-file", file, "-workers", "3", "-timeout", "1m")
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			for range 3 {
				<-started // all three workers are waiting on a site
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
			log := strings.Join(lines, "\n")

			if code := cmd.ProcessState.ExitCode(); code != 0 {
				t.Errorf("exit code = %d (%v), want 0", code, cmd.ProcessState)
			}
			if elapsed := time.Since(sent); elapsed > 2*time.Second {
				t.Errorf("exited %v after the signal, want it to cancel the checks at once", elapsed)
			}
			if !strings.Contains(log, "checks=3 up=0 down=0 timeouts=0 cancelled=3 skipped=3") {
				t.Errorf("want a round line with 3 cancelled and 3 skipped:\n%s", log)
			}
			if !strings.HasSuffix(log, " stopped") {
				t.Errorf("want the log to end with stopped:\n%s", log)
			}
		})
	}
}

// endregion: signals
