package observability

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// serve sends one GET through h and returns the status code the client sees.
func serve(h http.Handler) int {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	return rec.Code
}

var ok = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("ok\n")) // no WriteHeader: net/http sends 200
})

var broken = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "database unreachable", http.StatusInternalServerError)
})

// region: test

func TestInstrumentCountsRequests(t *testing.T) {
	reg := prometheus.NewRegistry()
	h := Instrument(reg, "status", ok)

	serve(h)
	serve(h)

	// The registry renders the counter in the text format Prometheus scrapes.
	want := `
# HELP http_requests_total HTTP requests answered, by handler and status code.
# TYPE http_requests_total counter
http_requests_total{code="200",handler="status"} 2
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(want), "http_requests_total"); err != nil {
		t.Error(err)
	}
	// One handler label value: one histogram series, holding both requests.
	n, err := testutil.GatherAndCount(reg, "http_request_duration_seconds")
	if err != nil || n != 1 {
		t.Errorf("http_request_duration_seconds has %d series, error %v, want 1", n, err)
	}
}

func TestInstrumentCountsServerErrors(t *testing.T) {
	reg := prometheus.NewRegistry()
	h := Instrument(reg, "monitors", broken)

	if code := serve(h); code != http.StatusInternalServerError {
		t.Fatalf("client got %d, want 500: Instrument must pass the status through", code)
	}
	want := `
# HELP http_requests_total HTTP requests answered, by handler and status code.
# TYPE http_requests_total counter
http_requests_total{code="500",handler="monitors"} 1
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(want), "http_requests_total"); err != nil {
		t.Error(err)
	}
}

// endregion: test

// TestInstrumentSharesOneRegistry wraps two handlers with one registry, as an
// api with several routes does. Registering the same metric twice must not
// panic, and each handler gets its own series.
func TestInstrumentSharesOneRegistry(t *testing.T) {
	reg := prometheus.NewRegistry()
	status := Instrument(reg, "status", ok)
	monitors := Instrument(reg, "monitors", broken)

	serve(status)
	serve(monitors)
	serve(monitors)

	want := `
# HELP http_requests_total HTTP requests answered, by handler and status code.
# TYPE http_requests_total counter
http_requests_total{code="200",handler="status"} 1
http_requests_total{code="500",handler="monitors"} 2
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(want), "http_requests_total"); err != nil {
		t.Error(err)
	}
	n, err := testutil.GatherAndCount(reg, "http_request_duration_seconds")
	if err != nil || n != 2 {
		t.Errorf("http_request_duration_seconds has %d series, error %v, want 2, one per handler", n, err)
	}
}
