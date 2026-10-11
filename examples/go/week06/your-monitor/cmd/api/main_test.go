package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// region: healthz

func TestHealthz(t *testing.T) {
	tests := []struct {
		method, path string
		wantStatus   int
	}{
		{http.MethodGet, "/healthz", http.StatusOK},
		{http.MethodPost, "/healthz", http.StatusMethodNotAllowed},
		{http.MethodGet, "/monitors", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newMux().ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantStatus != http.StatusOK {
				return
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			var body health
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("body is not JSON: %v", err)
			}
			if body.Status != "ok" {
				t.Errorf(`status field = %q, want "ok"`, body.Status)
			}
		})
	}
}

// endregion: healthz

// region: checkhealth

func TestCheckHealth(t *testing.T) {
	healthy := httptest.NewServer(newMux())
	t.Cleanup(healthy.Close)
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(failing.Close)
	gone := httptest.NewServer(newMux())
	gone.Close() // nothing listens on this address any more

	if err := checkHealth(healthy.URL + "/healthz"); err != nil {
		t.Errorf("healthy api: got %v, want nil", err)
	}
	if err := checkHealth(failing.URL + "/healthz"); err == nil {
		t.Error("api answering 503: got nil, want an error")
	}
	if err := checkHealth(gone.URL + "/healthz"); err == nil {
		t.Error("api not running: got nil, want an error")
	}
}

// endregion: checkhealth

// region: serve

func TestServeStopsWhenCancelled(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0") // port 0: the kernel picks a free port
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, ln, newMux()) }()

	url := "http://" + ln.Addr().String() + "/healthz"
	if err := checkHealth(url); err != nil {
		t.Fatalf("while serving: %v", err)
	}

	cancel() // what SIGTERM does in main
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("serve returned %v, want nil after a clean shutdown", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("serve still running 2 s after cancel")
	}
	if err := checkHealth(url); err == nil {
		t.Error("api still answers after shutdown")
	}
}

// endregion: serve
