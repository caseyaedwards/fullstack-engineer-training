package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSecureHeaders checks that every answer carries the security headers.
func TestSecureHeaders(t *testing.T) {
	h := newTestHandler(t)
	for _, path := range []string{"/healthz", "/api/monitors", "/api/monitors/1", "/no-such-page"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		for name, want := range map[string]string{
			"X-Content-Type-Options":  "nosniff",
			"X-Frame-Options":         "DENY",
			"Content-Security-Policy": "frame-ancestors 'none'",
		} {
			if got := rec.Header().Get(name); got != want {
				t.Errorf("GET %s: %s = %q, want %q", path, name, got, want)
			}
		}
	}
}
