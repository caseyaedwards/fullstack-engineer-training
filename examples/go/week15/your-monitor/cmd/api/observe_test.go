package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"fsp/examples/week15/your-monitor/internal/api"
	"fsp/examples/week15/your-monitor/internal/obs"
	"fsp/examples/week15/your-monitor/internal/obstest"
)

// region: order-test

// TestHandlerOrder sends one request through the whole chain of newHandler.
// Instrument and LogRequests sit inside Trace, which hands them the request
// the mux fills in: with them outside, every series reads route="unmatched".
func TestHandlerOrder(t *testing.T) {
	spans := obstest.RecordSpans(t)
	var logs bytes.Buffer
	old := slog.Default()
	slog.SetDefault(obs.NewLogger(&logs, "api"))
	t.Cleanup(func() { slog.SetDefault(old) })
	reg := prometheus.NewRegistry()
	h := newHandler(nil, nil, api.NewMetrics(reg))

	const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	req := httptest.NewRequest("GET", "/healthz", nil)
	req.Header.Set("traceparent", "00-"+traceID+"-00f067aa0ba902b7-01")
	h.ServeHTTP(httptest.NewRecorder(), req)

	want := `
# HELP http_requests_total HTTP requests answered, by method, route and status code.
# TYPE http_requests_total counter
http_requests_total{code="200",method="GET",route="/healthz"} 1
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(want), "http_requests_total"); err != nil {
		t.Error(err)
	}

	var line struct {
		Msg     string `json:"msg"`
		Route   string `json:"route"`
		TraceID string `json:"trace_id"`
	}
	if err := json.Unmarshal(logs.Bytes(), &line); err != nil {
		t.Fatalf("log %q: %v", logs.String(), err)
	}
	if line.Msg != "request" || line.Route != "/healthz" || line.TraceID != traceID {
		t.Errorf("log line %+v, want msg request, route /healthz, trace_id %s", line, traceID)
	}

	span := obstest.Find(t, spans.GetSpans(), "GET /healthz")
	if got := span.SpanContext.TraceID().String(); got != traceID {
		t.Errorf("span trace ID %s, want the caller's %s", got, traceID)
	}
}

// endregion: order-test

// TestHandlerOrderMatters shows what the test above guards against:
// Instrument outside Trace never sees the pattern the mux sets.
func TestHandlerOrderMatters(t *testing.T) {
	reg := prometheus.NewRegistry()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {})
	h := api.NewMetrics(reg).Instrument(api.Trace(mux)) // wrong: Instrument outermost
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/healthz", nil))

	want := `
# HELP http_requests_total HTTP requests answered, by method, route and status code.
# TYPE http_requests_total counter
http_requests_total{code="200",method="GET",route="unmatched"} 1
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(want), "http_requests_total"); err != nil {
		t.Error(err)
	}
}
