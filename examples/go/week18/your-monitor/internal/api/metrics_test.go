package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// region: metrics-test

// TestMetrics sends requests through Instrument, then scrapes /metrics
// from a test server the way Prometheus does, and reads the counts.
func TestMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	h := NewMetrics(reg).Instrument(newTestHandler(t))

	serve(h, "GET", "/api/monitors/1", "", "")
	serve(h, "GET", "/api/monitors/1", "", "")
	serve(h, "GET", "/api/monitors/2", "", "") // the same route, a 404
	serve(h, "GET", "/wp-login.php", "", "")   // no route
	serve(h, "BREW", "/api/monitors", "", "")  // not an HTTP method

	srv := httptest.NewServer(promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	for _, want := range []string{
		`http_requests_total{code="200",method="GET",route="/api/monitors/{id}"} 2`,
		`http_requests_total{code="404",method="GET",route="/api/monitors/{id}"} 1`,
		`http_requests_total{code="404",method="GET",route="unmatched"} 1`,
		`http_requests_total{code="405",method="OTHER",route="unmatched"} 1`,
		`http_request_duration_seconds_count{method="GET",route="/api/monitors/{id}"} 3`,
	} {
		if !strings.Contains(string(body), want+"\n") {
			t.Errorf("/metrics has no line %s", want)
		}
	}
	// One series per route, never one per path.
	if n := testutil.CollectAndCount(reg, "http_requests_total"); n != 4 {
		t.Errorf("http_requests_total has %d series, want 4", n)
	}
}

// endregion: metrics-test

func TestRouteWithoutMethod(t *testing.T) {
	r := httptest.NewRequest("GET", "/x", nil)
	r.Pattern = "/x/"
	if got := route(r); got != "/x/" {
		t.Errorf("route %q, want /x/", got)
	}
}
