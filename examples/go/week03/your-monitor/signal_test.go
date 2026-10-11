package main

import (
	"bufio"
	"context"
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

// region: signals

// TestProgramSignals builds the monitor, runs it against a test server,
// and sends it a signal once it has logged two checks.
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

// endregion: signals
