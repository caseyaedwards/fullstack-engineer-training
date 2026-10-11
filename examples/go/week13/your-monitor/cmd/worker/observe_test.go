package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"fsp/examples/week13/your-monitor/internal/db"
	"fsp/examples/week13/your-monitor/internal/db/dbtest"
	"fsp/examples/week13/your-monitor/internal/obs"
	"fsp/examples/week13/your-monitor/internal/obstest"
	"fsp/examples/week13/your-monitor/internal/queue"
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

// TestCheckLine pins the fields of the check line: the job and monitor IDs,
// the trace ID of the check span, and err for a site that does not answer.
func TestCheckLine(t *testing.T) {
	logs := captureLog(t)
	spans := obstest.RecordSpans(t)
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer broken.Close()
	gone := httptest.NewServer(nil)
	gone.Close() // connection refused

	check := newCheckFunc(2*time.Second, newCheckMetrics(prometheus.NewRegistry()))
	check(context.Background(), queue.Job{ID: 7, MonitorID: 3, URL: broken.URL})
	check(context.Background(), queue.Job{ID: 8, MonitorID: 4, URL: gone.URL})

	var lines []map[string]any
	dec := json.NewDecoder(logs)
	for dec.More() {
		var line map[string]any
		if err := dec.Decode(&line); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, line)
	}
	got := spans.GetSpans()
	if len(lines) != 2 || len(got) != 2 {
		t.Fatalf("%d check lines and %d spans, want 2 of each:\n%s", len(lines), len(got), logs)
	}
	// Spans end in call order, and JSON numbers decode as float64.
	for i, want := range []struct {
		job, monitor float64
		status       float64
		up, hasErr   bool
	}{
		{7, 3, 503, false, false},
		{8, 4, 0, false, true},
	} {
		line := lines[i]
		if line["msg"] != "check" || line["job_id"] != want.job || line["monitor_id"] != want.monitor ||
			line["status"] != want.status || line["up"] != want.up {
			t.Errorf("line %d = %v, want job_id %v, monitor_id %v, status %v, up %v", i, line, want.job, want.monitor, want.status, want.up)
		}
		if id := got[i].SpanContext.TraceID().String(); line["trace_id"] != id {
			t.Errorf("line %d trace_id = %v, want %s, the trace of its check span", i, line["trace_id"], id)
		}
		msg, hasErr := line["err"].(string)
		if hasErr != want.hasErr || (hasErr && msg == "") {
			t.Errorf("line %d err = %v, want an error text: %v", i, line["err"], want.hasErr)
		}
	}
}

// region: trace-test

// TestJobTrace runs one job through queue.Work on a pool that traces SQL,
// and checks the trace: a job span, with the check and the statements
// that store its result inside it.
func TestJobTrace(t *testing.T) {
	captureLog(t)
	url := dbtest.URL(t)
	dbtest.Pool(t, url, db.Migrate) // an empty schema with every migration
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
	// SAVEPOINT and RELEASE are the savepoint of Week 9 around the check.
	const want = "SAVEPOINT check INSERT DELETE RELEASE COMMIT job"
	if strings.Join(names, " ") != want {
		t.Fatalf("spans %q, want %s", names, want)
	}
	job := obstest.Find(t, got, "job")
	if job.Parent.IsValid() {
		t.Error("the job span has a parent; it must start the trace")
	}
	for _, s := range got[:len(got)-1] {
		if s.Parent.SpanID() != job.SpanContext.SpanID() || s.SpanContext.TraceID() != job.SpanContext.TraceID() {
			t.Errorf("span %s is not a child of the job span", s.Name)
		}
	}
	c := obstest.Find(t, got, "check")
	if a := obstest.Attrs(c); a["check.result"].AsString() != "down" || a["http.response.status_code"].AsInt64() != 503 {
		t.Errorf("check span attributes %v, want result down, status 503", c.Attributes)
	}
	if c.SpanKind != trace.SpanKindClient {
		t.Errorf("check span kind %v, want client: it covers a call to another program", c.SpanKind)
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
