package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// region: headers-test

// TestSecureHeaders checks that every answer carries the security headers,
// errors included: a JSON 404 and the 403 that sameOrigin sends to a
// cross-site write. It reads them from rec.Result(), which holds the
// headers as they were when the handler sent them. rec.Header() is the
// live map: it also shows headers set after the answer went out, which no
// client ever receives.
func TestSecureHeaders(t *testing.T) {
	h := newTestHandler(t)
	for _, tt := range []struct{ method, path, site string }{
		{http.MethodGet, "/healthz", ""},
		{http.MethodGet, "/api/monitors", ""},
		{http.MethodGet, "/api/monitors/1", ""},
		{http.MethodGet, "/no-such-page", ""},
		{http.MethodDelete, "/api/monitors/1", "cross-site"},
	} {
		req := httptest.NewRequest(tt.method, tt.path, nil)
		if tt.site != "" {
			req.Header.Set("Sec-Fetch-Site", tt.site)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		sent := rec.Result().Header
		for _, hdr := range []struct{ name, want string }{
			{"X-Content-Type-Options", "nosniff"},
			{"X-Frame-Options", "DENY"},
			{"Content-Security-Policy", "frame-ancestors 'none'"},
		} {
			if got := sent.Get(hdr.name); got != hdr.want {
				t.Errorf("%s %s: %s = %q, want %q", tt.method, tt.path, hdr.name, got, hdr.want)
			}
		}
	}
}

// endregion: headers-test
