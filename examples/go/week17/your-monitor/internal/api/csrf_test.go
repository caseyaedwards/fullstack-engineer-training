package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fsp/examples/week17/your-monitor/internal/monitor"
)

// region: csrf-test

// TestCrossOriginWritesAreRefused sends logged-in requests as a browser
// labels them. httptest requests go to the host example.com.
func TestCrossOriginWritesAreRefused(t *testing.T) {
	tests := []struct {
		name, method, path, header, value string
		want                              int
	}{
		{"page on this origin", "POST", "/api/logout", "Sec-Fetch-Site", "same-origin", 204},
		{"other site", "POST", "/api/logout", "Sec-Fetch-Site", "cross-site", 403},
		{"sibling subdomain", "POST", "/api/logout", "Sec-Fetch-Site", "same-site", 403},
		{"other site, delete", "DELETE", "/api/monitors/1", "Sec-Fetch-Site", "cross-site", 403},
		{"other site, read", "GET", "/api/me", "Sec-Fetch-Site", "cross-site", 200},
		{"old browser, this origin", "POST", "/api/logout", "Origin", "http://example.com", 204},
		{"old browser, other origin", "POST", "/api/logout", "Origin", "https://evil.example", 403},
		{"sandboxed page", "POST", "/api/logout", "Origin", "null", 403},
		{"curl", "POST", "/api/logout", "", "", 204},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var loggedOut string
			store := monitor.NewMemStore()
			store.Create(t.Context(), monitor.Monitor{URL: "https://example.com", IntervalSeconds: 60})
			h := NewHandler(store, nil, nil, fakeAuth{loggedOut: &loggedOut})

			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.AddCookie(&http.Cookie{Name: "session", Value: goodToken})
			if tt.header != "" {
				req.Header.Set(tt.header, tt.value)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Errorf("status %d, want %d; body %s", rec.Code, tt.want, rec.Body)
			}
			if tt.want == 403 {
				if !strings.Contains(rec.Body.String(), "cross-origin request refused") {
					t.Errorf("body %s", rec.Body)
				}
				list, _ := store.List(t.Context())
				if loggedOut != "" || len(list) != 1 {
					t.Error("the refused request ran the handler")
				}
			}
		})
	}
}

// endregion: csrf-test
