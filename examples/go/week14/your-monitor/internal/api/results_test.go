package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fsp/examples/week14/your-monitor/internal/monitor"
)

// spyResults is a ResultStore that records the limit it is asked for and
// returns no rows.
type spyResults struct{ limit int }

func (s *spyResults) Insert(context.Context, monitor.Result) error { return nil }
func (s *spyResults) Recent(_ context.Context, _ int64, limit int) ([]monitor.Result, error) {
	s.limit = limit
	return nil, nil
}

// region: test

func TestListResults(t *testing.T) {
	store := monitor.NewMemStore()
	m, _ := store.Create(context.Background(), monitor.Monitor{URL: "https://example.com", IntervalSeconds: 60})
	if m.ID != 1 {
		t.Fatalf("first monitor has ID %d, want 1", m.ID)
	}

	tests := []struct {
		path       string
		wantStatus int
		wantLimit  int    // the limit Recent receives; 0 when it is not called
		wantBody   string // a part of the body
	}{
		{"/api/monitors/1/results", 200, 50, "[]"},
		{"/api/monitors/1/results?limit=500", 200, 500, "[]"},
		{"/api/monitors/1/results?limit=1", 200, 1, "[]"},
		{"/api/monitors/1/results?limit=0", 400, 0, `"error":"limit must be an integer from 1 to 500`},
		{"/api/monitors/1/results?limit=501", 400, 0, `"error":"limit must be an integer from 1 to 500`},
		{"/api/monitors/1/results?limit=ten", 400, 0, `"error":"limit must be an integer from 1 to 500`},
		{"/api/monitors/2/results", 404, 0, `"error":"monitor 2 not found"`},
		{"/api/monitors/one/results", 400, 0, `"error":"id must be an integer, got`},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			spy := &spyResults{}
			rec := httptest.NewRecorder()
			NewHandler(store, spy, nil, fakeAuth{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want it to contain %q", rec.Body, tt.wantBody)
			}
			if spy.limit != tt.wantLimit {
				t.Errorf("Recent got limit %d, want %d", spy.limit, tt.wantLimit)
			}
		})
	}
}

// endregion: test
