package pool_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime/pprof"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	probe "fsp/examples/week04"
	"fsp/examples/week05/your-monitor/check"
	"fsp/examples/week05/your-monitor/pool"
)

// region: helpers

// checkWithin returns the check that main.go passes to pool.Run: probe.Check
// with a deadline of timeout on each check.
func checkWithin(timeout time.Duration) pool.CheckFunc {
	return func(ctx context.Context, target string) check.Result {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		timings, status, err := probe.Check(ctx, target)
		return check.Result{URL: target, Status: status, Latency: timings.FirstByte, Err: err, Timings: timings}
	}
}

// gauge counts the requests in flight and remembers the highest count.
type gauge struct{ now, peak atomic.Int64 }

func (g *gauge) enter() {
	n := g.now.Add(1)
	for {
		p := g.peak.Load()
		if n <= p || g.peak.CompareAndSwap(p, n) {
			return
		}
	}
}

func (g *gauge) leave() { g.now.Add(-1) }

// serve starts a test server that the test closes when it ends.
func serve(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

// endregion: helpers

// region: leaks

// poolGoroutines returns the stacks of the goroutines that run code from
// pool.Run, from the same goroutine dump that /debug/pprof/goroutine serves.
func poolGoroutines() []string {
	var buf bytes.Buffer
	pprof.Lookup("goroutine").WriteTo(&buf, 2)
	var stacks []string
	for _, g := range strings.Split(buf.String(), "\n\n") {
		if strings.Contains(g, "pool.Run.func") {
			stacks = append(stacks, g)
		}
	}
	return stacks
}

// checkNoLeaks fails the test when goroutines from pool.Run are still
// alive five seconds after Run returned.
func checkNoLeaks(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		left := poolGoroutines()
		if len(left) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d goroutines from pool.Run are still running:\n\n%s", len(left), strings.Join(left, "\n\n"))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// endregion: leaks

// region: hundred

func TestRunChecksEveryURLOnce(t *testing.T) {
	const workers = 8
	const timeout = 250 * time.Millisecond

	var inFlight gauge
	ok := serve(t, func(w http.ResponseWriter, r *http.Request) {
		inFlight.enter()
		defer inFlight.leave()
		time.Sleep(5 * time.Millisecond)
	})
	failing := serve(t, func(w http.ResponseWriter, r *http.Request) {
		inFlight.enter()
		defer inFlight.leave()
		time.Sleep(5 * time.Millisecond)
		w.WriteHeader(http.StatusInternalServerError)
	})
	// A slow handler answers only after the client gives up, which ends the
	// request context. It stays out of the gauge: it can still be running
	// for a moment after its check has returned.
	slow := serve(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
		}
	})

	// 100 URLs: every tenth is slow and every tenth fails; the rest are up.
	var targets []string
	wantTimeouts := map[string]bool{}
	for i := range 100 {
		srv := ok
		switch i % 10 {
		case 3:
			srv = slow
		case 7:
			srv = failing
		}
		u := fmt.Sprintf("%s/%d", srv.URL, i)
		targets = append(targets, u)
		if srv == slow {
			wantTimeouts[u] = true
		}
	}

	results := pool.Run(context.Background(), targets, workers, checkWithin(timeout))

	seen := map[string]int{}
	up, timeouts := 0, 0
	for _, r := range results {
		seen[r.URL]++
		if r.Up() {
			up++
		}
		if r.TimedOut() {
			timeouts++
			if !wantTimeouts[r.URL] {
				t.Errorf("%s timed out, want only the slow URLs to time out", r.URL)
			}
		}
	}
	for _, u := range targets {
		if seen[u] != 1 {
			t.Errorf("%s has %d results, want 1", u, seen[u])
		}
	}
	if len(results) != len(targets) {
		t.Errorf("got %d results, want %d", len(results), len(targets))
	}
	if up != 80 || timeouts != 10 {
		t.Errorf("up = %d, timeouts = %d; want 80 and 10", up, timeouts)
	}
	if peak := inFlight.peak.Load(); peak > workers || peak < 2 {
		t.Errorf("peak of %d requests in flight at once, want 2 to %d", peak, workers)
	}
	checkNoLeaks(t)
}

// endregion: hundred

// region: cancel

func TestRunStopsPromptlyWhenCancelled(t *testing.T) {
	const workers = 5

	// Every request blocks until the client gives up. The handler reports
	// each request it receives on a buffered channel, so it never waits for
	// the test to read.
	started := make(chan string, 100)
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		started <- r.URL.Path
		<-r.Context().Done()
	})
	var targets []string
	for i := range 100 {
		targets = append(targets, fmt.Sprintf("%s/%d", srv.URL, i))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan []check.Result, 1)
	go func() { done <- pool.Run(ctx, targets, workers, checkWithin(time.Minute)) }()

	for range workers {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatalf("only some of the %d workers started a check within 5 s", workers)
		}
	}
	cancel() // every worker is mid-check: SIGTERM arrives now

	var results []check.Result
	select {
	case results = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return within 10 s of the cancel, want it to stop at once")
	}
	if len(results) != workers {
		t.Errorf("got %d results, want %d: one per check in flight, none for the rest", len(results), workers)
	}
	for _, r := range results {
		if !errors.Is(r.Err, context.Canceled) {
			t.Errorf("%s: err = %v, want context.Canceled", r.URL, r.Err)
		}
	}
	checkNoLeaks(t)
}

// endregion: cancel

func TestRunPanicsWithoutWorkers(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("Run with 0 workers did not panic")
		}
	}()
	pool.Run(context.Background(), []string{"http://127.0.0.1:1/"}, 0, checkWithin(time.Second))
}
