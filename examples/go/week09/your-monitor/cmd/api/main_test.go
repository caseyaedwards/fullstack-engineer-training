package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"fsp/examples/week09/your-monitor/internal/api"
	"fsp/examples/week09/your-monitor/internal/db"
	"fsp/examples/week09/your-monitor/internal/db/dbtest"
	"fsp/examples/week09/your-monitor/internal/monitor"
)

// memHandler is the api over in-memory stores, for the tests that need a
// running api but no database.
func memHandler() http.Handler {
	return api.NewHandler(monitor.NewMemStore(), noResults{}, nil)
}

// noResults is a ResultStore that holds nothing.
type noResults struct{}

func (noResults) Insert(context.Context, monitor.Result) error { return nil }
func (noResults) Recent(context.Context, int64, int) ([]monitor.Result, error) {
	return nil, nil
}

// region: checkhealth

func TestCheckHealth(t *testing.T) {
	healthy := httptest.NewServer(memHandler())
	t.Cleanup(healthy.Close)
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(failing.Close)
	gone := httptest.NewServer(memHandler())
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
	go func() { done <- serve(ctx, ln, memHandler()) }()

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

// region: postgres

// TestAPIOnPostgres runs the api the way main wires it: openDB migrates
// an empty schema, and the handler reads and writes real tables.
func TestAPIOnPostgres(t *testing.T) {
	ctx := context.Background()
	pool, err := openDB(ctx, dbtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	srv := httptest.NewServer(newHandler(pool))
	t.Cleanup(srv.Close)

	resp, err := http.Post(srv.URL+"/api/monitors", "application/json",
		strings.NewReader(`{"url": "https://example.com", "interval_seconds": 60}`))
	if err != nil {
		t.Fatal(err)
	}
	var m monitor.Monitor
	json.NewDecoder(resp.Body).Decode(&m)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/monitors: status %d, want 201", resp.StatusCode)
	}
	location := resp.Header.Get("Location") // /api/monitors/1

	// Three checks, one minute apart, as the worker writes them.
	results := db.NewResultStore(pool)
	start := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	for i := range 3 {
		status := 200 + i
		err := results.Insert(ctx, monitor.Result{
			MonitorID: m.ID, CheckedAt: start.Add(time.Duration(i) * time.Minute),
			Up: true, StatusCode: &status,
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	var got []monitor.Result
	getJSON(t, srv.URL+location+"/results?limit=2", http.StatusOK, &got)
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2", len(got))
	}
	if *got[0].StatusCode != 202 || *got[1].StatusCode != 201 {
		t.Errorf("status codes %d, %d: want 202, 201, newest first", *got[0].StatusCode, *got[1].StatusCode)
	}
	if got[0].DNSMs != nil {
		t.Errorf("dns_ms = %d, want null: the insert left it out", *got[0].DNSMs)
	}

	// Deleting the monitor deletes its results too: on delete cascade.
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+location, nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	var left int
	if err := pool.QueryRow(ctx, "select count(*) from check_results").Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Errorf("%d results left after DELETE, want 0", left)
	}
	getJSON(t, srv.URL+location+"/results", http.StatusNotFound, nil)
}

// endregion: postgres

func getJSON(t *testing.T, url string, wantStatus int, v any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		t.Fatalf("GET %s: status %d, want %d", url, resp.StatusCode, wantStatus)
	}
	if v != nil {
		if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
			t.Fatalf("GET %s: %v", url, err)
		}
	}
}
