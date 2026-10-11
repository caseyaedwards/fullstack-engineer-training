package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"fsp/examples/week02/your-monitor/check"
)

// openFiles counts the file descriptors this process holds. It reads
// /proc/self/fd, which exists only on Linux.
func openFiles(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Skip("needs /proc/self/fd, which only Linux has")
	}
	return len(entries)
}

// TestUnclosedBodiesLeakFileDescriptors checks the claim on the Week 3 page:
// every response body left open holds a connection, and with it a file
// descriptor, while check.URL closes each body and holds none.
func TestUnclosedBodiesLeakFileDescriptors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)
	client := &http.Client{}
	t.Cleanup(client.CloseIdleConnections)

	before := openFiles(t)
	for range 20 {
		check.URL(client, srv.URL)
	}
	if grown := openFiles(t) - before; grown > 4 {
		t.Errorf("check.URL: open files grew by %d over 20 checks, want 4 or fewer", grown)
	}

	before = openFiles(t)
	for range 20 {
		resp, err := client.Get(srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { resp.Body.Close() })
	}
	// Each open body holds one socket in the client. The test server
	// lives in the same process and holds the other end of each one.
	if grown := openFiles(t) - before; grown < 20 {
		t.Errorf("unclosed bodies: open files grew by %d over 20 requests, want 20 or more", grown)
	}
}
