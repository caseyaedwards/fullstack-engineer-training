package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.opentelemetry.io/otel/codes"

	"fsp/examples/week15/your-monitor/internal/dbtest"
	"fsp/examples/week15/your-monitor/internal/obs"
	"fsp/examples/week15/your-monitor/internal/obstest"
	"fsp/examples/week15/your-monitor/internal/queue"
)

// region: metrics-test

// TestCheckMetrics runs the check function against a site that answers
// 200, one that answers 503 and one where nothing listens, then compares
// the counter with what Prometheus scrapes.
func TestCheckMetrics(t *testing.T) {
	captureLog(t)
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ok.Close()
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer broken.Close()
	gone := httptest.NewServer(nil)
	gone.Close() // connection refused

	reg := prometheus.NewRegistry()
	check := newCheckFunc(2*time.Second, newCheckMetrics(reg))
	for _, url := range []string{ok.URL, ok.URL, broken.URL, gone.URL} {
		check(context.Background(), queue.Job{ID: 1, MonitorID: 1, URL: url})
	}

	want := `
# HELP monitor_checks_total Checks run, by result: up, down (the site answered with an error status) or error (no answer).
# TYPE monitor_checks_total counter
monitor_checks_total{result="down"} 1
monitor_checks_total{result="error"} 1
monitor_checks_total{result="up"} 2
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(want), "monitor_checks_total"); err != nil {
		t.Error(err)
	}
	if n := histogramCount(t, reg, "monitor_check_duration_seconds"); n != 4 {
		t.Errorf("monitor_check_duration_seconds counted %d checks, want 4", n)
	}
}

// endregion: metrics-test

// histogramCount returns how many observations the histogram name holds.
func histogramCount(t *testing.T, reg *prometheus.Registry, name string) uint64 {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.GetName() == name {
			return f.GetMetric()[0].GetHistogram().GetSampleCount()
		}
	}
	t.Fatalf("no metric %s", name)
	return 0
}

// region: trace-test

// TestJobTrace runs one job through queue.Work on a pool that traces SQL,
// and checks the trace: a job span, with the check and the statements
// that store its result inside it.
func TestJobTrace(t *testing.T) {
	captureLog(t)
	_, url := dbtest.NewWithURL(t)
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Tracer = obs.QueryTracer{}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer broken.Close()
	dbtest.Exec(t, pool, `INSERT INTO monitors (url, interval_seconds) VALUES ($1, 60)`, broken.URL)
	if _, err := queue.Enqueue(context.Background(), pool); err != nil {
		t.Fatal(err)
	}

	spans := obstest.RecordSpans(t)
	check := newCheckFunc(2*time.Second, newCheckMetrics(prometheus.NewRegistry()))
	if worked, err := queue.Work(context.Background(), pool, check); !worked || err != nil {
		t.Fatalf("Work = %v, %v; want true, nil", worked, err)
	}

	got := spans.GetSpans()
	var names []string
	for _, s := range got {
		names = append(names, s.Name)
	}
	// Spans are exported as they end: the children first, the job last.
	if strings.Join(names, " ") != "check INSERT DELETE COMMIT job" {
		t.Fatalf("spans %q, want check INSERT DELETE COMMIT job", names)
	}
	job := obstest.Find(t, got, "job")
	if job.Parent.IsValid() {
		t.Error("the job span has a parent; it must start the trace")
	}
	for _, s := range got[:4] {
		if s.Parent.SpanID() != job.SpanContext.SpanID() || s.SpanContext.TraceID() != job.SpanContext.TraceID() {
			t.Errorf("span %s is not a child of the job span", s.Name)
		}
	}
	c := obstest.Find(t, got, "check")
	if a := obstest.Attrs(c); a["check.result"].AsString() != "down" || a["http.response.status_code"].AsInt64() != 503 {
		t.Errorf("check span attributes %v, want result down, status 503", c.Attributes)
	}
	if c.Status.Code == codes.Error {
		t.Error("a site that answers 503 is a successful check: the check span must not be an error")
	}

	// A poll of the empty queue makes no trace at all.
	spans.Reset()
	if worked, _ := queue.Work(context.Background(), pool, check); worked {
		t.Fatal("Work found a second job")
	}
	if n := len(spans.GetSpans()); n != 0 {
		t.Errorf("a poll of the empty queue made %d spans, want 0", n)
	}
}

// endregion: trace-test
