package obs_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"fsp/examples/week18/your-monitor/internal/obs"
)

// region: health-test

// TestHealthGoesStaleWithoutBeats is what the liveness probe sees: ok
// while the loop beats, 503 once it stops for longer than maxAge, and ok
// again after the next beat.
func TestHealthGoesStaleWithoutBeats(t *testing.T) {
	h := obs.NewHealth(100 * time.Millisecond)
	probe := func() (int, string) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		return rec.Code, rec.Body.String()
	}

	if code, body := probe(); code != http.StatusOK || body != "{\"status\":\"ok\"}\n" {
		t.Fatalf("fresh: %d %q, want 200 {\"status\":\"ok\"}", code, body)
	}
	time.Sleep(300 * time.Millisecond) // no Beat: the loop is stuck
	if code, body := probe(); code != http.StatusServiceUnavailable || !strings.Contains(body, `"status":"stuck"`) {
		t.Fatalf("stale: %d %q, want 503 with status stuck", code, body)
	}
	h.Beat()
	if code, _ := probe(); code != http.StatusOK {
		t.Fatalf("after Beat: %d, want 200", code)
	}
}

// endregion: health-test

func TestOpsMuxServesHealthAndMetrics(t *testing.T) {
	srv := httptest.NewServer(obs.OpsMux(obs.NewHealth(time.Minute)))
	defer srv.Close()
	for path, want := range map[string]string{"/healthz": `{"status":"ok"}`, "/metrics": "go_goroutines"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), want) {
			t.Errorf("GET %s: %d, body without %q", path, resp.StatusCode, want)
		}
	}
	resp, err := http.Post(srv.URL+"/healthz", "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST /healthz: %d, want 405", resp.StatusCode)
	}
}
