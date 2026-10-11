package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandler(t *testing.T) {
	var s stats
	srv := httptest.NewServer(s.handler(50 * time.Millisecond))
	t.Cleanup(srv.Close)

	tests := []struct {
		path       string
		wantStatus int
		atLeast    time.Duration
	}{
		{"/ok/1", http.StatusOK, 0},
		{"/fail/2", http.StatusInternalServerError, 0},
		{"/slow/3", http.StatusOK, 50 * time.Millisecond},
	}
	for _, tt := range tests {
		start := time.Now()
		resp, err := http.Get(srv.URL + tt.path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != tt.wantStatus {
			t.Errorf("%s: status %d, want %d", tt.path, resp.StatusCode, tt.wantStatus)
		}
		if took := time.Since(start); took < tt.atLeast {
			t.Errorf("%s: answered after %v, want at least %v", tt.path, took, tt.atLeast)
		}
	}
	if got := s.requests.Load(); got != 3 {
		t.Errorf("requests = %d, want 3", got)
	}
	if got := s.peak.Load(); got != 1 {
		t.Errorf("peak = %d, want 1: the requests ran one after another", got)
	}
}
