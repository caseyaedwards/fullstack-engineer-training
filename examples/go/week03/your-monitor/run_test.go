package main

import (
	"bytes"
	"context"
	"log"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"fsp/examples/week02/your-monitor/check"
)

// region: helpers

// captureLog sends the log package to a buffer for the length of the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &buf
}

// endregion: helpers

// region: rounds

func TestRunChecksEveryTargetEachRound(t *testing.T) {
	captureLog(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var checked []string
	checkOne := func(target string) check.Result {
		checked = append(checked, target)
		if len(checked) == 6 {
			cancel() // the last check of round 3, as if SIGTERM arrived now
		}
		return check.Result{URL: target, Status: 200}
	}
	run(ctx, []string{"https://example.com", "https://example.org"}, time.Millisecond, checkOne)

	want := []string{
		"https://example.com", "https://example.org",
		"https://example.com", "https://example.org",
		"https://example.com", "https://example.org",
	}
	if !slices.Equal(checked, want) {
		t.Errorf("checked %q, want %q", checked, want)
	}
}

// endregion: rounds

// region: shutdown

func TestRunSkipsTheRestOfTheRound(t *testing.T) {
	logs := captureLog(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var checked []string
	checkOne := func(target string) check.Result {
		checked = append(checked, target)
		cancel() // SIGTERM arrives while the first check runs
		return check.Result{URL: target, Status: 200}
	}
	run(ctx, []string{"https://example.com", "https://example.org", "https://example.net"}, time.Hour, checkOne)

	if !slices.Equal(checked, []string{"https://example.com"}) {
		t.Errorf("checked %q, want only the check that had started", checked)
	}
	if !strings.Contains(logs.String(), "skipped 2 of 3 checks") {
		t.Errorf("log = %q, want it to report the 2 skipped checks", logs)
	}
}

func TestRunStopsWaitingWhenCancelled(t *testing.T) {
	captureLog(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(50*time.Millisecond, cancel) // SIGTERM arrives between rounds

	calls := 0
	checkOne := func(target string) check.Result {
		calls++
		return check.Result{URL: target, Status: 200}
	}
	start := time.Now()
	run(ctx, []string{"https://example.com"}, time.Hour, checkOne)

	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("run returned after %v, want it to stop waiting at once", elapsed)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

// endregion: shutdown
