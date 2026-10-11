// region: imports

package check_test

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"fsp/examples/week02/your-monitor/check"
)

// endregion: imports

// region: table

func TestResult(t *testing.T) {
	tests := []struct {
		name   string
		result check.Result
		wantUp bool
		want   string
	}{
		{
			name:   "ok",
			result: check.Result{URL: "https://example.com", Status: 200, Latency: 118 * time.Millisecond},
			wantUp: true,
			want:   "up url=https://example.com status=200 latency_ms=118",
		},
		{
			name:   "399 is the last status that counts as up",
			result: check.Result{URL: "https://example.com", Status: 399, Latency: 40 * time.Millisecond},
			wantUp: true,
			want:   "up url=https://example.com status=399 latency_ms=40",
		},
		{
			name:   "400 is the first status that counts as down",
			result: check.Result{URL: "https://example.com", Status: 400, Latency: 40 * time.Millisecond},
			wantUp: false,
			want:   "down url=https://example.com status=400 latency_ms=40",
		},
		{
			name:   "server error",
			result: check.Result{URL: "https://example.com", Status: 503, Latency: 9 * time.Millisecond},
			wantUp: false,
			want:   "down url=https://example.com status=503 latency_ms=9",
		},
		{
			name:   "no response",
			result: check.Result{URL: "http://127.0.0.1:1/", Err: errors.New("connection refused")},
			wantUp: false,
			want:   `down url=http://127.0.0.1:1/ latency_ms=0 error="connection refused"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.Up(); got != tt.wantUp {
				t.Errorf("Up() = %v, want %v", got, tt.wantUp)
			}
			if got := tt.result.String(); got != tt.want {
				t.Errorf("String() = %q\nwant       %q", got, tt.want)
			}
		})
	}
}

// endregion: table

// region: server

// newServer starts a test server that waits for delay, then answers
// with status. It closes the server when the test ends.
func newServer(t *testing.T, status int, delay time.Duration) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(delay):
		case <-r.Context().Done(): // the client gave up
			return
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestURLAgainstATestServer(t *testing.T) {
	tests := []struct {
		name   string
		status int
		wantUp bool
	}{
		{"ok", http.StatusOK, true},
		{"not found", http.StatusNotFound, false},
		{"server error", http.StatusInternalServerError, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newServer(t, tt.status, 0)
			client := &http.Client{Timeout: time.Second}

			got := check.URL(client, srv.URL)
			if got.Err != nil {
				t.Fatalf("Err = %v, want nil", got.Err)
			}
			if got.URL != srv.URL {
				t.Errorf("URL = %q, want %q", got.URL, srv.URL)
			}
			if got.Status != tt.status {
				t.Errorf("Status = %d, want %d", got.Status, tt.status)
			}
			if got.Up() != tt.wantUp {
				t.Errorf("Up() = %v, want %v", got.Up(), tt.wantUp)
			}
		})
	}
}

func TestURLGivesUpAtTheClientTimeout(t *testing.T) {
	srv := newServer(t, http.StatusOK, 10*time.Second)
	timeout := 100 * time.Millisecond
	client := &http.Client{Timeout: timeout}

	got := check.URL(client, srv.URL)
	var netErr net.Error
	if !errors.As(got.Err, &netErr) || !netErr.Timeout() {
		t.Fatalf("Err = %v, want a timeout error", got.Err)
	}
	if got.Up() {
		t.Error("Up() = true for a check that timed out")
	}
	if got.Latency < timeout || got.Latency > 2*time.Second {
		t.Errorf("Latency = %v, want about the %v timeout", got.Latency, timeout)
	}
}

// endregion: server

// region: fake

// fakeGetter satisfies check.Getter without a network. It hands back
// the response and error it holds.
type fakeGetter struct {
	resp *http.Response
	err  error
}

func (f fakeGetter) Get(url string) (*http.Response, error) {
	return f.resp, f.err
}

// spyBody is a response body that records whether it was closed.
type spyBody struct {
	io.Reader
	closed bool
}

func (b *spyBody) Close() error {
	b.closed = true
	return nil
}

func TestURLClosesTheBody(t *testing.T) {
	body := &spyBody{Reader: strings.NewReader("hello")}
	client := fakeGetter{resp: &http.Response{StatusCode: 200, Body: body}}

	got := check.URL(client, "https://example.com")
	if !body.closed {
		t.Error("URL returned without closing the response body")
	}
	if got.Status != 200 || got.Err != nil {
		t.Errorf("got status %d, error %v; want 200 and nil", got.Status, got.Err)
	}
}

func TestURLReportsTheErrorFromGet(t *testing.T) {
	refused := errors.New("dial tcp 127.0.0.1:1: connect: connection refused")
	client := fakeGetter{err: refused}

	got := check.URL(client, "http://127.0.0.1:1/")
	if !errors.Is(got.Err, refused) {
		t.Errorf("Err = %v, want %v", got.Err, refused)
	}
	if got.Status != 0 || got.Up() {
		t.Errorf("Status = %d, Up() = %v; want 0 and false", got.Status, got.Up())
	}
}

// endregion: fake
