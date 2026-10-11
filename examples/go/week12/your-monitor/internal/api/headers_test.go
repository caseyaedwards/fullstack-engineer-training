package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// region: headers-test

// TestSecureHeaders checks that every answer carries the security headers.
// It reads them from rec.Result(), which holds the headers as they were
// when the handler sent them. rec.Header() is the live map: it also shows
// headers set after the answer went out, which no client ever receives.
func TestSecureHeaders(t *testing.T) {
	h := newTestHandler(t)
	for _, path := range []string{"/healthz", "/api/monitors", "/api/monitors/1", "/no-such-page"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		sent := rec.Result().Header
		for name, want := range map[string]string{
			"X-Content-Type-Options":  "nosniff",
			"X-Frame-Options":         "DENY",
			"Content-Security-Policy": "frame-ancestors 'none'",
		} {
			if got := sent.Get(name); got != want {
				t.Errorf("GET %s: %s = %q, want %q", path, name, got, want)
			}
		}
	}
}

// endregion: headers-test
