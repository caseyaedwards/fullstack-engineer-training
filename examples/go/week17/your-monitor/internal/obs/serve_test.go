package obs_test

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"fsp/examples/week17/your-monitor/internal/obs"
)

// region: serve-test

// TestNewServerDropsSlowClients connects, sends half a request and stops.
// The server must close the connection once ReadHeaderTimeout runs out.
// The test shortens the timeout to keep the run fast.
func TestNewServerDropsSlowClients(t *testing.T) {
	srv := obs.NewServer("", http.NotFoundHandler())
	if srv.ReadHeaderTimeout <= 0 {
		t.Fatal("NewServer sets no ReadHeaderTimeout")
	}
	srv.ReadHeaderTimeout = 200 * time.Millisecond

	ln, err := net.Listen("tcp", "127.0.0.1:0") // port 0: the kernel picks a free port
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(ln) //nolint:errcheck // Serve returns ErrServerClosed when the test closes srv
	t.Cleanup(func() { srv.Close() })

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := io.WriteString(conn, "GET / HTTP/1.1\r\nHost: example"); err != nil {
		t.Fatal(err)
	}

	// Read blocks until the server closes the connection, or until this
	// deadline, which is far longer than the server timeout.
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	start := time.Now()
	_, err = io.ReadAll(conn)
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		t.Fatal("the server kept the slow connection open")
	}
	if waited := time.Since(start); waited > 2*time.Second {
		t.Fatalf("the server closed the connection after %v, want about 200ms", waited)
	}
}

// endregion: serve-test

// region: pprof-test

// TestNewPprofServerWantsLoopback checks that pprof never listens on an
// address that other machines reach.
func TestNewPprofServerWantsLoopback(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:6061", "[::1]:6061", "localhost:6061"} {
		if _, err := obs.NewPprofServer(addr); err != nil {
			t.Errorf("NewPprofServer(%q): %v, want a server", addr, err)
		}
	}
	for _, addr := range []string{":6061", "0.0.0.0:6061", "[::]:6061", "10.0.0.5:6061", "worker:6061", "127.0.0.1"} {
		if _, err := obs.NewPprofServer(addr); err == nil {
			t.Errorf("NewPprofServer(%q) returned a server, want an error", addr)
		}
	}
}

// TestNewPprofServerServesProfiles checks that the pages answer on the
// pprof server, from a mux of its own.
func TestNewPprofServerServesProfiles(t *testing.T) {
	srv, err := obs.NewPprofServer("127.0.0.1:6061")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/debug/pprof/", "/debug/pprof/goroutine?debug=1", "/debug/pprof/cmdline"} {
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s: status %d, want 200", path, rec.Code)
		}
	}
	if srv.Handler == http.DefaultServeMux {
		t.Error("the pprof server serves http.DefaultServeMux, want a mux of its own")
	}
}

// endregion: pprof-test
