package networking

import (
	"errors"
	"io"
	"net"
	"os"
	"syscall"
	"testing"
	"time"
)

// start runs Serve on a free port of 127.0.0.1 and returns its address.
// The cleanup closes the listener and checks that Serve returns nil.
func start(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- Serve(ln) }()
	t.Cleanup(func() {
		ln.Close()
		if err := <-done; err != nil {
			t.Errorf("Serve after Close = %v, want nil", err)
		}
	})
	return ln.Addr().String()
}

// region: test
func TestEchoRoundTrip(t *testing.T) {
	addr := start(t)
	got, err := Echo(addr, "hello", 2*time.Second)
	if err != nil {
		t.Fatalf("Echo: %v", err)
	}
	if got != "hello" {
		t.Errorf("Echo = %q, want %q", got, "hello")
	}
}

func TestEchoClosedPortIsRefused(t *testing.T) {
	// Take a free port, then close it: nothing listens there now.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	_, err = Echo(addr, "hello", 2*time.Second)
	if !errors.Is(err, syscall.ECONNREFUSED) {
		t.Fatalf("Echo to a closed port: err = %v, want ECONNREFUSED", err)
	}
}

// endregion: test

// A server that accepts nothing still completes the TCP handshake from
// its backlog, but never answers. Echo gives up at its deadline.
func TestEchoTimesOutOnASilentServer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	start := time.Now()
	_, err = Echo(ln.Addr().String(), "hello", 100*time.Millisecond)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Echo took %v, want well under 2s for a 100ms timeout", elapsed)
	}
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("err = %v, want os.ErrDeadlineExceeded", err)
	}
}

// A client that connects and says nothing loses its connection after
// the two-second idle deadline.
func TestServeClosesAnIdleConnection(t *testing.T) {
	addr := start(t)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	begin := time.Now()
	_, err = conn.Read(make([]byte, 1))
	elapsed := time.Since(begin)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("Read on an idle connection: err = %v, want io.EOF from the server closing it", err)
	}
	if elapsed < time.Second || elapsed > 6*time.Second {
		t.Errorf("server closed the idle connection after %v, want about 2s", elapsed)
	}
}

// Each Echo opens its own connection, and the server handles them in turn.
func TestServeHandlesManyConnections(t *testing.T) {
	addr := start(t)
	for _, line := range []string{"one", "two", "three"} {
		got, err := Echo(addr, line, 2*time.Second)
		if err != nil || got != line {
			t.Errorf("Echo(%q) = %q, %v", line, got, err)
		}
	}
}
