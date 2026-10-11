package check_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	probe "fsp/examples/week04"
	"fsp/examples/week05/your-monitor/check"
)

// checkWithin runs probe.Check against url with a deadline of d, as the
// monitor does, and returns the Result.
func checkWithin(ctx context.Context, url string, d time.Duration) check.Result {
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	timings, status, err := probe.Check(ctx, url)
	return check.Result{URL: url, Status: status, Latency: timings.FirstByte, Err: err, Timings: timings}
}

// TestTimedOut checks that every way a check can run out of time counts as
// a timeout, and that a refused connection, an error status and a
// cancelled check do not.
func TestTimedOut(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	wait := func(r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}

	slowHeaders := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wait(r)
	}))
	t.Cleanup(slowHeaders.Close)
	slowBody := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("first part"))
		w.(http.Flusher).Flush()
		wait(r)
	}))
	t.Cleanup(slowBody.Close)
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(failing.Close)

	// A listener that accepts the TCP connection and never answers the TLS
	// ClientHello: the check stalls in the handshake.
	silent, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { silent.Close() })
	go func() {
		for {
			conn, err := silent.Accept()
			if err != nil {
				return
			}
			go func() { io.Copy(io.Discard, conn); conn.Close() }()
		}
	}()

	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close() // nothing listens on this port any more: connection refused

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name          string
		ctx           context.Context
		url           string
		wantTimedOut  bool
		wantCancelled bool
	}{
		{"headers never arrive", context.Background(), slowHeaders.URL, true, false},
		{"body stops halfway", context.Background(), slowBody.URL, true, false},
		{"TLS handshake stalls", context.Background(), "https://" + silent.Addr().String(), true, false},
		{"error status", context.Background(), failing.URL, false, false},
		{"connection refused", context.Background(), gone.URL, false, false},
		{"cancelled before it starts", cancelled, slowHeaders.URL, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := checkWithin(tt.ctx, tt.url, 100*time.Millisecond)
			if r.TimedOut() != tt.wantTimedOut {
				t.Errorf("TimedOut() = %v, want %v (err: %v)", r.TimedOut(), tt.wantTimedOut, r.Err)
			}
			if r.Cancelled() != tt.wantCancelled {
				t.Errorf("Cancelled() = %v, want %v (err: %v)", r.Cancelled(), tt.wantCancelled, r.Err)
			}
		})
	}
}

// TestListIsUnchanged keeps list.go a copy of the Week 2 file, whose
// tests cover ParseList.
func TestListIsUnchanged(t *testing.T) {
	week2, err := os.ReadFile(filepath.Join("..", "..", "..", "week02", "your-monitor", "check", "list.go"))
	if err != nil {
		t.Fatal(err)
	}
	ours, err := os.ReadFile("list.go")
	if err != nil {
		t.Fatal(err)
	}
	if string(ours) != string(week2) {
		t.Error("list.go differs from week02/your-monitor/check/list.go")
	}
}
