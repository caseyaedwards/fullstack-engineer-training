package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"fsp/examples/week13/your-monitor/internal/api"
)

// region: checkhealth

func TestCheckHealth(t *testing.T) {
	healthy := httptest.NewServer(newHandler(nil, nil, api.NewMetrics(prometheus.NewRegistry())))
	t.Cleanup(healthy.Close)
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(failing.Close)
	gone := httptest.NewServer(newHandler(nil, nil, api.NewMetrics(prometheus.NewRegistry())))
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
	go func() { done <- serve(ctx, ln, newHandler(nil, nil, api.NewMetrics(prometheus.NewRegistry()))) }()

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
