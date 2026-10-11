package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"fsp/examples/week18/your-monitor/internal/alert"
)

// fakeIncidents returns one open and one resolved incident, and records
// what the handler asked for.
type fakeIncidents struct{ askedOpenOnly *bool }

func (f fakeIncidents) List(ctx context.Context, openOnly bool) ([]alert.Incident, error) {
	*f.askedOpenOnly = openOnly
	start := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	end := start.Add(5 * time.Minute)
	open := alert.Incident{ID: 2, MonitorID: 1, URL: "https://example.com", StartedAt: start.Add(time.Hour), Cause: "status 503"}
	if openOnly {
		return []alert.Incident{open}, nil
	}
	return []alert.Incident{open, {ID: 1, MonitorID: 1, URL: "https://example.com", StartedAt: start, ResolvedAt: &end, Cause: "status 500"}}, nil
}

// region: incidents-test

func TestListIncidents(t *testing.T) {
	tests := []struct {
		query        string
		wantStatus   int
		wantOpenOnly bool
		wantBody     string
	}{
		{"", 200, false, `"resolved_at":"2026-10-11T09:05:00Z"`},
		{"?open=true", 200, true, `[{"id":2,"monitor_id":1,"url":"https://example.com","started_at":"2026-10-11T10:00:00Z","resolved_at":null,"cause":"status 503"}]`},
		{"?open=false", 200, false, `"id":1`},
		{"?open=yes", 400, false, `{"error":"open must be true or false"}`},
	}
	for _, tt := range tests {
		var openOnly bool
		h := NewHandler(nil, nil, fakeIncidents{askedOpenOnly: &openOnly}, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/incidents"+tt.query, nil))

		if rec.Code != tt.wantStatus || openOnly != tt.wantOpenOnly {
			t.Errorf("GET /api/incidents%s: status %d, openOnly %v; want %d, %v",
				tt.query, rec.Code, openOnly, tt.wantStatus, tt.wantOpenOnly)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("GET /api/incidents%s: Content-Type %q", tt.query, ct)
		}
		if !strings.Contains(rec.Body.String(), tt.wantBody) {
			t.Errorf("GET /api/incidents%s: body %s, want it to contain %s", tt.query, rec.Body, tt.wantBody)
		}
	}
}

// endregion: incidents-test

func TestListIncidentsIsAnEmptyArrayNotNull(t *testing.T) {
	h := NewHandler(nil, nil, emptyIncidents{}, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/incidents?open=true", nil))
	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Errorf("body %s, want []", got)
	}
}

type emptyIncidents struct{}

func (emptyIncidents) List(context.Context, bool) ([]alert.Incident, error) {
	return []alert.Incident{}, nil
}
