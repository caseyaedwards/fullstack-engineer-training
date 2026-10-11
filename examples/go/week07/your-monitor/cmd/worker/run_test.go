package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fsp/examples/week07/your-monitor/internal/check"
)

// region: helpers

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

// endregion: helpers

// region: rounds

func TestRunLogsASummaryEachRound(t *testing.T) {
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
	targets := []string{"https://example.com", "https://slow.example", "https://broken.example"}
	run(ctx, targets, time.Millisecond, 2, checkOne)

	if got := calls.Load(); got != 6 {
		t.Errorf("checkOne called %d times, want 6: two rounds of three", got)
	}
	want := "round checks=3 up=1 down=2 timeouts=1 cancelled=0 skipped=0"
	if n := strings.Count(logs.String(), want); n != 2 {
		t.Errorf("found %d lines with %q, want 2\n%s", n, want, logs)
	}
}

// endregion: rounds

// region: shutdown

func TestRunCancelsTheChecksInFlight(t *testing.T) {
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
	targets := []string{"https://a.example", "https://b.example", "https://c.example", "https://d.example", "https://e.example"}
	done := make(chan struct{})
	go func() {
		run(ctx, targets, time.Hour, 2, checkOne)
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
	case <-time.After(10 * time.Second):
		t.Fatal("run did not return within 10 s of the cancel, want it to stop at once")
	}
	want := "round checks=2 up=0 down=0 timeouts=0 cancelled=2 skipped=3"
	if !strings.Contains(logs.String(), want) {
		t.Errorf("log = %q, want a line with %q", logs, want)
	}
}

// TestRunStopsWaitingWhenCancelled is the Week 3 test, with the new
// checkOne: SIGTERM arrives while run waits an hour for the next round.
func TestRunStopsWaitingWhenCancelled(t *testing.T) {
	captureLog(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(50*time.Millisecond, cancel) // SIGTERM arrives between rounds

	var calls atomic.Int64
	checkOne := func(ctx context.Context, target string) check.Result {
		calls.Add(1)
		return check.Result{URL: target, Status: 200}
	}
	start := time.Now()
	run(ctx, []string{"https://example.com"}, time.Hour, 2, checkOne)

	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("run returned after %v, want it to stop waiting at once", elapsed)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("calls = %d, want 1", got)
	}
}

// endregion: shutdown

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
