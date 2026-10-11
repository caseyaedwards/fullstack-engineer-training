package main

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fsp/examples/week11/your-monitor/internal/check"
	"fsp/examples/week11/your-monitor/internal/db"
	"fsp/examples/week11/your-monitor/internal/db/dbtest"
	"fsp/examples/week11/your-monitor/internal/probe"
	"fsp/examples/week11/your-monitor/internal/queue"
)

// captureLog sends the log package to a buffer for the length of the test.
// log.Logger is safe to call from several goroutines; bytes.Buffer is not,
// but the logger takes a lock around every write.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &buf
}

// region: run-test

// TestRunDrainsThenPolls gives run 30 fake jobs. The workers take jobs back
// to back while there are any, then wait poll between looks at the empty
// queue, and all stop when ctx is cancelled.
func TestRunDrainsThenPolls(t *testing.T) {
	captureLog(t)
	ctx, cancel := context.WithCancel(context.Background())
	var jobs, calls atomic.Int64
	jobs.Store(30)
	work := func(ctx context.Context) (bool, error) {
		calls.Add(1)
		return jobs.Add(-1) >= 0, nil // false once the 30 jobs are gone
	}

	done := make(chan struct{})
	go func() {
		run(ctx, 3, 50*time.Millisecond, work)
		close(done)
	}()
	time.Sleep(120 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("run did not return within a second of the cancel")
	}

	// 30 jobs, plus at most three looks per worker at the empty queue in
	// 120 ms with a 50 ms poll: at 0, 50 and 100 ms.
	if got := calls.Load(); got < 33 || got > 39 {
		t.Errorf("work called %d times, want 33 to 39", got)
	}
}

// endregion: run-test

// region: worker-test

// TestWorkerChecksQueuedJobs runs the real check function against two
// test sites through the real queue: one answers 200, one 503.
func TestWorkerChecksQueuedJobs(t *testing.T) {
	logs := captureLog(t)
	pool := dbtest.Pool(t, dbtest.URL(t), db.Migrate)
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ok.Close()
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer broken.Close()
	dbtest.Exec(t, pool, `INSERT INTO monitors (url, interval_seconds) VALUES ($1, 60), ($2, 60)`, ok.URL, broken.URL)
	if _, err := queue.Enqueue(context.Background(), pool); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	checkJob := newCheckFunc(2 * time.Second)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		run(ctx, 2, 10*time.Millisecond, func(ctx context.Context) (bool, error) {
			return queue.Work(ctx, pool, checkJob)
		})
	}()
	deadline := time.Now().Add(5 * time.Second)
	for dbtest.Count(t, pool, `SELECT count(*) FROM check_jobs`) > 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-stopped // the workers write the log: wait for them before reading it

	got := dbtest.Count(t, pool, `SELECT count(*) FROM check_results r JOIN monitors m ON m.id = r.monitor_id
		WHERE (m.url = $1 AND r.up AND r.status_code = 200) OR (m.url = $2 AND NOT r.up AND r.status_code = 503)`,
		ok.URL, broken.URL)
	if got != 2 {
		t.Errorf("found %d matching results, want one up with 200 and one down with 503\nlogs:\n%s", got, logs)
	}
	if !strings.Contains(logs.String(), "status=503") {
		t.Errorf("no check line for the 503 site in the logs:\n%s", logs)
	}
}

// endregion: worker-test

// region: toresult

// TestToResult is the Week 8 test: the worker still turns each check into
// a row the same way.
func TestToResult(t *testing.T) {
	checkedAt := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	up := toResult(7, check.Result{
		URL: "http://203.0.113.10/", CheckedAt: checkedAt, Status: 200,
		// An IP address over plain http: no DNS, no TLS.
		Timings: probe.Timings{Connect: 11800 * time.Microsecond, FirstByte: 52600 * time.Microsecond, Total: 53 * time.Millisecond},
	})
	if up.MonitorID != 7 || !up.CheckedAt.Equal(checkedAt) || !up.Up || *up.StatusCode != 200 {
		t.Errorf("toResult = %+v, want monitor 7, up, status 200 at %v", up, checkedAt)
	}
	if up.DNSMs != nil || up.TLSMs != nil || up.Error != nil {
		t.Error("dns_ms, tls_ms and error must be NULL for an http check of an IP address")
	}
	if *up.ConnectMs != 11 || *up.FirstByteMs != 52 || *up.TotalMs != 53 {
		t.Errorf("connect, first byte, total = %d, %d, %d ms, want 11, 52, 53", *up.ConnectMs, *up.FirstByteMs, *up.TotalMs)
	}

	down := toResult(7, check.Result{URL: "https://no-such-host.example", Err: errors.New("no such host"),
		Timings: probe.Timings{DNS: 400 * time.Microsecond}})
	if down.Up || down.StatusCode != nil || *down.Error != "no such host" {
		t.Errorf("toResult = %+v, want down, status NULL, the error text", down)
	}
	if down.DNSMs == nil || *down.DNSMs != 0 {
		t.Error("a 0.4 ms DNS lookup happened: want dns_ms 0, not NULL")
	}
}

// endregion: toresult
