package api

import (
	"errors"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"fsp/examples/week18/your-monitor/internal/obstest"
)

// region: trace-test

func TestTraceNamesTheSpanAfterTheRoute(t *testing.T) {
	spans := obstest.RecordSpans(t)
	h := Trace(newTestHandler(t))

	serve(h, "GET", "/api/monitors/1", "", "")

	got := spans.GetSpans()
	if len(got) != 1 {
		t.Fatalf("got %d spans, want 1", len(got))
	}
	s := got[0]
	if s.Name != "GET /api/monitors/{id}" || s.SpanKind != trace.SpanKindServer {
		t.Errorf("span %q kind %v, want GET /api/monitors/{id}, server", s.Name, s.SpanKind)
	}
	a := obstest.Attrs(s)
	if a["http.request.method"].AsString() != "GET" || a["http.route"].AsString() != "/api/monitors/{id}" ||
		a["http.response.status_code"].AsInt64() != 200 {
		t.Errorf("span attributes %v, want method GET, route /api/monitors/{id}, status 200", s.Attributes)
	}
}

// TestTraceJoinsTheCallerTrace sends a traceparent header, as a caller
// that traces its own work does. The api span must join that trace, as a
// child of the caller span.
func TestTraceJoinsTheCallerTrace(t *testing.T) {
	spans := obstest.RecordSpans(t)
	h := Trace(newTestHandler(t))

	const traceID, parentID = "4bf92f3577b34da6a3ce929d0e0e4736", "00f067aa0ba902b7"
	req := httptest.NewRequest("GET", "/api/monitors", nil)
	req.Header.Set("traceparent", "00-"+traceID+"-"+parentID+"-01")
	h.ServeHTTP(httptest.NewRecorder(), req)

	s := spans.GetSpans()[0]
	if s.SpanContext.TraceID().String() != traceID || s.Parent.SpanID().String() != parentID {
		t.Errorf("span in trace %s with parent %s, want trace %s, parent %s",
			s.SpanContext.TraceID(), s.Parent.SpanID(), traceID, parentID)
	}
}

// endregion: trace-test

func TestTraceMarksServerErrors(t *testing.T) {
	spans := obstest.RecordSpans(t)
	h := Trace(NewHandler(brokenStore{err: errors.New("connection refused")}, nil, nil, fakeAuth{}))
	serve(h, "GET", "/api/monitors", "", "")
	if s := spans.GetSpans()[0]; s.Status.Code != codes.Error {
		t.Errorf("span status %v after a 500, want Error", s.Status)
	}
}
