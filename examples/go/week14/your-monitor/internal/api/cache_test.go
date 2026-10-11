package api

import (
	"net/http"
	"testing"
)

// region: cache-test

// TestAnswersAreNotStored checks the Cache-Control header of two reads the
// status page polls, of the logged-in user and of a write. no-store keeps
// every answer out of the browser cache: the next poll asks the api again.
func TestAnswersAreNotStored(t *testing.T) {
	h := newTestHandler(t)
	for _, tt := range []struct{ method, path string }{
		{http.MethodGet, "/api/monitors"},
		{http.MethodGet, "/api/monitors/1"},
		{http.MethodGet, "/api/me"},
		{http.MethodDelete, "/api/monitors/1"},
	} {
		rec := serve(h, tt.method, tt.path, "", "")
		if got := rec.Result().Header.Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s %s: Cache-Control = %q, want no-store", tt.method, tt.path, got)
		}
	}
}

// endregion: cache-test
