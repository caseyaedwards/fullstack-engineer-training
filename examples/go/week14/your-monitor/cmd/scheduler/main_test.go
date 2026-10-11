package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"fsp/examples/week14/your-monitor/internal/dbtest"
	"fsp/examples/week14/your-monitor/internal/obs"
)

// region: round-test

func TestRoundLogsAndMeasuresTheQueue(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(obs.NewLogger(&logs, "scheduler"))
	t.Cleanup(func() { slog.SetDefault(prev) })
	db := dbtest.New(t)
	dbtest.Exec(t, db, `INSERT INTO monitors (url, interval_seconds) VALUES ('https://example.com', 60), ('https://example.org', 60)`)
	m := newQueueMetrics(prometheus.NewRegistry())

	round(context.Background(), db, m)
	round(context.Background(), db, m)

	dec := json.NewDecoder(&logs)
	for _, want := range []float64{2, 0} {
		var line map[string]any
		if err := dec.Decode(&line); err != nil {
			t.Fatal(err)
		}
		if line["msg"] != "round" || line["enqueued"] != want || line["queued"] != 2.0 {
			t.Errorf("log line %v, want round with enqueued=%v queued=2", line, want)
		}
	}
	if got := testutil.ToFloat64(m.queued); got != 2 {
		t.Errorf("monitor_jobs_queued = %v, want 2", got)
	}
	// Both jobs were queued with run_at = now() during the first round.
	if got := testutil.ToFloat64(m.oldest); got <= 0 || got > 5 {
		t.Errorf("monitor_jobs_oldest_seconds = %v, want a few milliseconds", got)
	}
}

// endregion: round-test

func TestEveryStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	every(ctx, 1, func(context.Context) {
		calls++
		if calls == 3 {
			cancel()
		}
	})
	if calls != 3 {
		t.Errorf("f called %d times, want 3", calls)
	}
}
