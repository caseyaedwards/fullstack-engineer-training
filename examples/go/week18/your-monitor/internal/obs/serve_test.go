package obs_test

import (
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"fsp/examples/week18/your-monitor/internal/obs"
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
