package main

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"fsp/examples/week14/your-monitor/internal/dbtest"
	"fsp/examples/week14/your-monitor/internal/obs"
	"fsp/examples/week14/your-monitor/internal/queue"
)

// captureLog sends the default logger to a buffer, as JSON, for the
// length of the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf syncBuffer
	prev := slog.Default()
	slog.SetDefault(obs.NewLogger(&buf, "worker"))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf.Buffer
}

// syncBuffer is a bytes.Buffer that several goroutines can write to.
type syncBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Write(p)
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
	db := dbtest.New(t)
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ok.Close()
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer broken.Close()
	dbtest.Exec(t, db, `INSERT INTO monitors (url, interval_seconds) VALUES ($1, 60), ($2, 60)`, ok.URL, broken.URL)
	if _, err := queue.Enqueue(context.Background(), db); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	check := newCheckFunc(2*time.Second, newCheckMetrics(prometheus.NewRegistry()))
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		run(ctx, 2, 10*time.Millisecond, func(ctx context.Context) (bool, error) {
			return queue.Work(ctx, db, check)
		})
	}()
	deadline := time.Now().Add(5 * time.Second)
	for dbtest.Count(t, db, `SELECT count(*) FROM check_jobs`) > 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-stopped // the workers write the log: wait for them before reading it

	got := dbtest.Count(t, db, `SELECT count(*) FROM check_results r JOIN monitors m ON m.id = r.monitor_id
		WHERE (m.url = $1 AND r.up AND r.status_code = 200) OR (m.url = $2 AND NOT r.up AND r.status_code = 503)`,
		ok.URL, broken.URL)
	if got != 2 {
		t.Errorf("found %d matching results, want one up with 200 and one down with 503\nlogs:\n%s", got, logs)
	}
	if !strings.Contains(logs.String(), `"up":false,"status":503`) {
		t.Errorf("no check line for the 503 site in the logs:\n%s", logs)
	}
}

// endregion: worker-test
