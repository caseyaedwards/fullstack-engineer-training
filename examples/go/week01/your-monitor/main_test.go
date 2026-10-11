package main

import (
	"bytes"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// server starts a test server that waits for delay, then answers with status.
// A delay longer than the client timeout ends early when the client gives up,
// so the test does not wait for the full delay.
func server(t *testing.T, status int, delay time.Duration) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
			return
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCheckURLReportsStatusAndLatency(t *testing.T) {
	srv := server(t, http.StatusOK, 50*time.Millisecond)
	client := &http.Client{Timeout: time.Second}

	status, latency, err := checkURL(client, srv.URL)
	if err != nil {
		t.Fatalf("checkURL: %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("status = %d, want 200", status)
	}
	if latency < 50*time.Millisecond || latency >= time.Second {
		t.Errorf("latency = %v, want at least the 50ms the server waits, and under the timeout", latency)
	}
}

func TestCheckURLReturnsAServerErrorAsAStatusNotAnError(t *testing.T) {
	srv := server(t, http.StatusInternalServerError, 0)
	client := &http.Client{Timeout: time.Second}

	status, _, err := checkURL(client, srv.URL)
	// A 500 is an answer: the request worked, and the site says it is broken.
	if err != nil {
		t.Fatalf("checkURL: %v, want nil error for a 500 response", err)
	}
	if status != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", status)
	}
}

func TestCheckURLGivesUpAtTheClientTimeout(t *testing.T) {
	srv := server(t, http.StatusOK, 10*time.Second)
	timeout := 100 * time.Millisecond
	client := &http.Client{Timeout: timeout}

	status, latency, err := checkURL(client, srv.URL)
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("err = %v, want a timeout error", err)
	}
	if !strings.Contains(err.Error(), "Client.Timeout exceeded while awaiting headers") {
		t.Errorf("err = %q, want the message the page shows", err)
	}
	if status != 0 {
		t.Errorf("status = %d, want 0 when no response arrives", status)
	}
	if latency < timeout || latency > 2*time.Second {
		t.Errorf("latency = %v, want about the %v timeout", latency, timeout)
	}
}

func TestCheckURLRejectsAURLWithoutAScheme(t *testing.T) {
	client := &http.Client{Timeout: time.Second}
	_, _, err := checkURL(client, "example.com")
	if err == nil || !strings.Contains(err.Error(), `unsupported protocol scheme ""`) {
		t.Errorf("err = %v, want unsupported protocol scheme", err)
	}
}

// TestExitCodes builds the program and runs it the way the page does, to check
// what it prints and the exit code a shell sees.
func TestExitCodes(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "your-monitor")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	up := server(t, http.StatusOK, 0)
	down := server(t, http.StatusInternalServerError, 0)
	slow := server(t, http.StatusOK, 10*time.Second)

	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string
	}{
		{"up", []string{"-url", up.URL}, 0, "up url=" + up.URL + " status=200 latency_ms="},
		{"server error", []string{"-url", down.URL}, 1, "down url=" + down.URL + " status=500 latency_ms="},
		{"timeout", []string{"-url", slow.URL, "-timeout", "200ms"}, 1, `(Client.Timeout exceeded while awaiting headers)"`},
		{"no url", nil, 2, "missing -url"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := exec.Command(bin, tt.args...)
			cmd.Stdout = &out
			cmd.Stderr = &out
			err := cmd.Run()
			code := 0
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				code = exitErr.ExitCode()
			} else if err != nil {
				t.Fatalf("run: %v", err)
			}
			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d\n%s", code, tt.wantCode, out.String())
			}
			if !strings.Contains(out.String(), tt.wantOut) {
				t.Errorf("output = %q, want it to contain %q", out.String(), tt.wantOut)
			}
		})
	}
}
