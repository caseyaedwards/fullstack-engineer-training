package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fsp/examples/week08/your-monitor/internal/check"
	"fsp/examples/week08/your-monitor/internal/monitor"
	"fsp/examples/week08/your-monitor/internal/probe"
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

// region: fakes

// fakeMonitors is a lister that returns a fixed list.
type fakeMonitors []monitor.Monitor

func (f fakeMonitors) List(context.Context) ([]monitor.Monitor, error) { return f, nil }

// fakeResults is an inserter that keeps every row in memory.
type fakeResults struct {
	mu   sync.Mutex
	rows []monitor.Result
}

func (f *fakeResults) Insert(ctx context.Context, r monitor.Result) error {
	if err := ctx.Err(); err != nil {
		return err // a real database refuses a cancelled context too
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows = append(f.rows, r)
	return nil
}

// endregion: fakes

func monitors(urls ...string) fakeMonitors {
	var ms fakeMonitors
	for i, u := range urls {
		ms = append(ms, monitor.Monitor{ID: int64(i + 1), URL: u, IntervalSeconds: 60})
	}
	return ms
}

// region: rounds

func TestRunStoresOneRowPerCheck(t *testing.T) {
	logs := captureLog(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The pool calls the fake from several goroutines at once, which makes
	// a plain int counter a data race. An atomic counter is safe.
	var calls atomic.Int64
	checkOne := func(ctx context.Context, target string) check.Result {
		if calls.Add(1) == 6 {
			cancel() // the last check of round 2, as if SIGTERM arrived now
		}
		switch target {
		case "https://slow.example":
			return check.Result{URL: target, Err: fmt.Errorf("probe: %w", context.DeadlineExceeded)}
		case "https://broken.example":
			return check.Result{URL: target, Status: 500}
		}
		return check.Result{URL: target, Status: 200}
	}
	results := &fakeResults{}
	run(ctx, monitors("https://example.com", "https://slow.example", "https://broken.example"),
		results, time.Millisecond, 2, checkOne)

	want := "round checks=3 up=1 down=2 timeouts=1 cancelled=0 skipped=0"
	if n := strings.Count(logs.String(), want); n != 2 {
		t.Errorf("found %d lines with %q, want 2\n%s", n, want, logs)
	}
	// Six checks, six rows: the cancel came after the last check had
	// finished, and save still stores round 2.
	if len(results.rows) != 6 {
		t.Fatalf("stored %d rows, want 6", len(results.rows))
	}
	up := map[int64]int{}
	for _, r := range results.rows {
		if r.Up {
			up[r.MonitorID]++
		}
	}
	if up[1] != 2 || up[2] != 0 || up[3] != 0 {
		t.Errorf("up rows per monitor = %v, want monitor 1 up twice and the others never", up)
	}
}

// endregion: rounds

// region: shutdown

func TestRunDoesNotStoreCancelledChecks(t *testing.T) {
	logs := captureLog(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Every check reports that it started, then waits until it is
	// cancelled, like a site that never answers.
	started := make(chan string, 5)
	checkOne := func(ctx context.Context, target string) check.Result {
		started <- target
		<-ctx.Done()
		return check.Result{URL: target, Err: ctx.Err()}
	}
	results := &fakeResults{}
	done := make(chan struct{})
	go func() {
		run(ctx, monitors("https://a.example", "https://b.example", "https://c.example", "https://d.example", "https://e.example"),
			results, time.Hour, 2, checkOne)
		close(done)
	}()
	for range 2 { // both workers are mid-check
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("the two workers did not start their checks within 5 s")
		}
	}
	cancel() // SIGTERM arrives during the first round
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("run still running 1 s after cancel, want it to stop at once")
	}

	want := "round checks=2 up=0 down=0 timeouts=0 cancelled=2 skipped=3"
	if !strings.Contains(logs.String(), want) {
		t.Errorf("log = %q, want a line with %q", logs, want)
	}
	if len(results.rows) != 0 {
		t.Errorf("stored %d rows for cancelled checks, want 0", len(results.rows))
	}
}

// endregion: shutdown

// region: toresult

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

func TestSummarize(t *testing.T) {
	results := []check.Result{
		{URL: "https://a.example", Status: 200},
		{URL: "https://b.example", Status: 404},
		{URL: "https://c.example", Err: fmt.Errorf("get: %w", context.DeadlineExceeded)},
		{URL: "https://d.example", Err: fmt.Errorf("get: %w", context.Canceled)},
	}
	got := summarize(6, results).String()
	want := "checks=4 up=1 down=2 timeouts=1 cancelled=1 skipped=2"
	if got != want {
		t.Errorf("summarize = %q, want %q", got, want)
	}
}
