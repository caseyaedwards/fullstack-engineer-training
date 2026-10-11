package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"fsp/examples/week17/your-monitor/internal/monitor"
)

// pods is the k3s pod network, where Traefik and nginx run.
var pods = []netip.Prefix{netip.MustParsePrefix("10.42.0.0/16")}

// newLimited returns the api behind RateLimits with a clock that only
// moves when the test moves it.
func newLimited(t *testing.T, cfg RateConfig) (http.Handler, *RateLimits, *time.Time) {
	t.Helper()
	now := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	l := NewRateLimits(cfg, prometheus.NewRegistry())
	l.now = func() time.Time { return now }
	return l.Limit(NewHandler(monitor.NewMemStore(), nil, nil, fakeAuth{})), l, &now
}

// from sends method path to h as if Traefik passed it on from client.
func from(h http.Handler, client, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.RemoteAddr = "10.42.0.9:40000" // the Traefik pod
	req.Header.Set("X-Forwarded-For", client)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRateLimitPerClient(t *testing.T) {
	h, l, now := newLimited(t, RateConfig{Limit: 10, Burst: 4, Trusted: pods})

	for i := range 4 {
		if rec := from(h, "203.0.113.7", "GET", "/api/monitors", ""); rec.Code != http.StatusOK {
			t.Fatalf("request %d of the burst: status %d, want 200", i+1, rec.Code)
		}
	}
	rec := from(h, "203.0.113.7", "GET", "/api/monitors", "")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("fifth request: status %d, want 429", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Errorf("Retry-After = %q, want 1: 100 ms, rounded up", got)
	}
	var body errorBody
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil || body.Error != "too many requests" {
		t.Errorf("body %+v (%v), want {\"error\":\"too many requests\"}", body, err)
	}

	// Another client behind the same Traefik has its own bucket.
	if rec := from(h, "198.51.100.1", "GET", "/api/monitors", ""); rec.Code != http.StatusOK {
		t.Errorf("another client: status %d, want 200", rec.Code)
	}
	// 100 ms later, one token is back.
	*now = now.Add(100 * time.Millisecond)
	if rec := from(h, "203.0.113.7", "GET", "/api/monitors", ""); rec.Code != http.StatusOK {
		t.Errorf("after 100ms: status %d, want 200", rec.Code)
	}
	if got := testutil.ToFloat64(l.refused.WithLabelValues("client")); got != 1 {
		t.Errorf("http_requests_limited_total{limit=\"client\"} = %v, want 1", got)
	}
}

// region: spoof-test

// TestSpoofedHeaderDoesNotEscapeTheLimit: a client that talks to the api
// directly, past Traefik, writes a new X-Forwarded-For on every request.
// The api ignores it, because the peer is not a trusted proxy.
func TestSpoofedHeaderDoesNotEscapeTheLimit(t *testing.T) {
	h, _, _ := newLimited(t, RateConfig{Limit: 10, Burst: 2, Trusted: pods})
	codes := make([]int, 3)
	for i := range codes {
		req := httptest.NewRequest("GET", "/api/monitors", nil)
		req.RemoteAddr = "203.0.113.7:51000"
		req.Header.Set("X-Forwarded-For", netip.AddrFrom4([4]byte{198, 51, 100, byte(i)}).String())
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		codes[i] = rec.Code
	}
	if codes[2] != http.StatusTooManyRequests {
		t.Errorf("statuses %v, want the third request refused", codes)
	}
}

// endregion: spoof-test

// TestLoginLimits: the login limits apply even with the client limit off.
// Each client gets 5 logins at once; all clients together get 3 a second.
func TestLoginLimits(t *testing.T) {
	h, l, now := newLimited(t, RateConfig{Trusted: pods})
	login := `{"email":"you@example.com","password":"wrong"}`

	for i := range 3 {
		if rec := from(h, "203.0.113.7", "POST", "/api/login", login); rec.Code != http.StatusUnauthorized {
			t.Fatalf("login %d: status %d, want 401", i+1, rec.Code)
		}
	}
	// The fourth login in the same instant hits the cap for everyone,
	// whichever client sends it.
	rec := from(h, "198.51.100.1", "POST", "/api/login", login)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "1" {
		t.Errorf("fourth login from anyone: status %d, Retry-After %q, want 429 and 1", rec.Code, rec.Header().Get("Retry-After"))
	}
	// Reads are not logins, and stay open.
	if rec := from(h, "198.51.100.1", "GET", "/api/monitors", ""); rec.Code != http.StatusOK {
		t.Errorf("read during the login cap: status %d, want 200", rec.Code)
	}

	// Ten seconds later the shared bucket is full again. The first client
	// has 2 tokens left plus 10/12 of a new one: two logins pass, and the
	// next one waits the 2 seconds the rest of that token takes.
	*now = now.Add(10 * time.Second)
	for i := range 2 {
		if rec := from(h, "203.0.113.7", "POST", "/api/login", login); rec.Code != http.StatusUnauthorized {
			t.Fatalf("login %d after 10s: status %d, want 401", i+4, rec.Code)
		}
	}
	rec = from(h, "203.0.113.7", "POST", "/api/login", login)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "2" {
		t.Errorf("sixth login of one client: status %d, Retry-After %q, want 429 and 2", rec.Code, rec.Header().Get("Retry-After"))
	}
	if got := testutil.ToFloat64(l.refused.WithLabelValues("login")); got != 1 {
		t.Errorf("limited{limit=login} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(l.refused.WithLabelValues("login_all")); got != 1 {
		t.Errorf("limited{limit=login_all} = %v, want 1", got)
	}
}

// TestRequestLogNamesTheClient: LogRequests runs inside Limit and logs the
// address that Limit found, not the address of the proxy.
func TestRequestLogNamesTheClient(t *testing.T) {
	var buf bytes.Buffer
	l := NewRateLimits(RateConfig{Trusted: pods}, prometheus.NewRegistry())
	h := l.Limit(LogRequests(slog.New(slog.NewJSONHandler(&buf, nil)), NewHandler(monitor.NewMemStore(), nil, nil, fakeAuth{})))
	from(h, "203.0.113.7", "GET", "/healthz", "")

	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatal(err)
	}
	if line["client"] != "203.0.113.7" {
		t.Errorf("client = %v, want 203.0.113.7", line["client"])
	}
}
