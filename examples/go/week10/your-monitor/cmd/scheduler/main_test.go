package main

import (
	"bytes"
	"context"
	"log"
	"os"
	"regexp"
	"testing"

	"fsp/examples/week10/your-monitor/internal/db"
	"fsp/examples/week10/your-monitor/internal/db/dbtest"
)

func TestRoundLogsTheQueue(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	pool := dbtest.Pool(t, dbtest.URL(t), db.Migrate)
	dbtest.Exec(t, pool, `INSERT INTO monitors (url, interval_seconds) VALUES ('https://example.com', 60), ('https://example.org', 60)`)

	round(context.Background(), pool)
	round(context.Background(), pool)

	want := regexp.MustCompile(`(?m)round enqueued=2 queued=2 oldest_ms=\d+\n.*round enqueued=0 queued=2 oldest_ms=\d+\n`)
	if !want.MatchString(logs.String()) {
		t.Errorf("logs:\n%s\nwant two round lines: 2 enqueued, then 0, with 2 queued", logs.String())
	}
}

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
