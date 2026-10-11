package obs_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace/noop"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"

	"fsp/examples/week14/your-monitor/internal/obs"
	"fsp/examples/week14/your-monitor/internal/obstest"
)

// region: logger-test

func TestLoggerAddsTheTraceID(t *testing.T) {
	obstest.RecordSpans(t)
	var buf bytes.Buffer
	logger := obs.NewLogger(&buf, "worker").With("worker", 3)

	logger.Info("outside a span")
	ctx, span := otel.Tracer("test").Start(context.Background(), "job")
	logger.InfoContext(ctx, "inside a span")
	span.End()

	dec := json.NewDecoder(&buf)
	var outside, inside map[string]any
	if err := dec.Decode(&outside); err != nil {
		t.Fatal(err)
	}
	if err := dec.Decode(&inside); err != nil {
		t.Fatal(err)
	}
	if outside["service"] != "worker" || outside["worker"] != 3.0 || outside["trace_id"] != nil {
		t.Errorf("line outside a span: %v, want service and worker, no trace_id", outside)
	}
	if inside["trace_id"] != span.SpanContext().TraceID().String() ||
		inside["span_id"] != span.SpanContext().SpanID().String() {
		t.Errorf("line inside a span: %v, want trace_id %s", inside, span.SpanContext().TraceID())
	}
}

// endregion: logger-test

// region: sql-test

func TestQueryTracerMakesSpansOnlyInsideATrace(t *testing.T) {
	spans := obstest.RecordSpans(t)
	var tracer obs.QueryTracer
	query := func(ctx context.Context, sql string, err error) {
		ctx = tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{SQL: sql})
		tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{Err: err})
	}

	query(context.Background(), "SELECT 1", nil) // a poll outside any trace
	if n := len(spans.GetSpans()); n != 0 {
		t.Fatalf("a statement outside a trace made %d spans, want 0", n)
	}

	ctx, job := otel.Tracer("test").Start(context.Background(), "job")
	query(ctx, "\n  insert into check_results VALUES (1)", nil)
	query(ctx, "DELETE FROM check_jobs", errors.New("deadlock detected"))
	if n := len(spans.GetSpans()); n != 2 {
		t.Fatalf("got %d spans before the job ends, want 2: a statement ended its parent", n)
	}
	job.End()

	insert := obstest.Find(t, spans.GetSpans(), "INSERT")
	if insert.Parent.SpanID() != job.SpanContext().SpanID() {
		t.Error("the INSERT span is not a child of the job span")
	}
	if obstest.Attrs(insert)["db.system.name"].AsString() != "postgresql" {
		t.Errorf("INSERT attributes %v, want db.system.name postgresql", insert.Attributes)
	}
	if del := obstest.Find(t, spans.GetSpans(), "DELETE"); del.Status.Code != codes.Error {
		t.Errorf("a failed statement has status %v, want Error", del.Status)
	}
}

// endregion: sql-test

func TestStartTracingIsOffWithoutAnEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	shutdown, err := obs.StartTracing(context.Background(), "api")
	if err != nil {
		t.Fatal(err)
	}
	_, span := otel.Tracer("test").Start(context.Background(), "job")
	defer span.End()
	if span.IsRecording() {
		t.Error("with no endpoint, spans must not record")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// TestStartTracingSendsOTLP points the exporter at a test server that
// stands in for the collector, and checks that shutdown sends the waiting
// span there as an OTLP/HTTP request.
func TestStartTracingSendsOTLP(t *testing.T) {
	got := make(chan string, 10)
	services := make(chan string, 10)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Method + " " + r.URL.Path + " " + r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		var req collectortrace.ExportTraceServiceRequest
		if err := proto.Unmarshal(body, &req); err != nil {
			services <- "unreadable: " + err.Error()
			return
		}
		for _, rs := range req.ResourceSpans {
			for _, kv := range rs.Resource.Attributes {
				if kv.Key == "service.name" {
					services <- kv.Value.GetStringValue()
				}
			}
		}
	}))
	defer collector.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collector.URL)
	t.Cleanup(func() { otel.SetTracerProvider(noop.NewTracerProvider()) })

	shutdown, err := obs.StartTracing(context.Background(), "api")
	if err != nil {
		t.Fatal(err)
	}
	_, span := otel.Tracer("test").Start(context.Background(), "job")
	span.End()
	if err := shutdown(context.Background()); err != nil { // flushes the batch
		t.Fatal(err)
	}
	select {
	case req := <-got:
		if req != "POST /v1/traces application/x-protobuf" {
			t.Errorf("collector got %q, want POST /v1/traces application/x-protobuf", req)
		}
	default:
		t.Error("the collector got nothing")
	}
	select {
	case name := <-services:
		if name != "api" {
			t.Errorf("service.name %q, want api", name)
		}
	default:
		t.Error("the spans carry no service.name")
	}
}
