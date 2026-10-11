package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"fsp/examples/week13/your-monitor/internal/monitor"
)

// region: log-test

func TestLogRequests(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	h := LogRequests(logger, NewHandler(monitor.NewMemStore(), &spyResults{}, nil, fakeAuth{}))

	serve(h, "POST", "/api/monitors", "application/json", `{"url":"https://example.com","interval_seconds":60}`)
	serve(h, "DELETE", "/api/monitors/1", "", "")
	serve(h, "GET", "/api/monitors/7", "", "")
	serve(h, "GET", "/no/such/path", "", "")

	want := []struct {
		method, route, path string
		status              float64 // encoding/json decodes every JSON number as float64
		empty               bool    // the answer has no body: bytes is 0
	}{
		{"POST", "/api/monitors", "/api/monitors", 201, false},
		{"DELETE", "/api/monitors/{id}", "/api/monitors/1", 204, true},
		{"GET", "/api/monitors/{id}", "/api/monitors/7", 404, false},
		{"GET", "unmatched", "/no/such/path", 404, false},
	}
	dec := json.NewDecoder(&buf)
	for _, w := range want {
		var line map[string]any
		if err := dec.Decode(&line); err != nil {
			t.Fatalf("log line for %s %s: %v", w.method, w.path, err)
		}
		if line["level"] != "INFO" || line["msg"] != "request" || line["method"] != w.method ||
			line["route"] != w.route || line["path"] != w.path || line["status"] != w.status {
			t.Errorf("got %v, want %s %s route=%s status=%v", line, w.method, w.path, w.route, w.status)
		}
		if n, ok := line["bytes"].(float64); !ok || (n == 0) != w.empty {
			t.Errorf("bytes = %v in %v, want %s", line["bytes"], line, map[bool]string{true: "0", false: "more than 0"}[w.empty])
		}
		if _, ok := line["duration_ms"].(float64); !ok {
			t.Errorf("no duration_ms number in %v", line)
		}
	}
}

// endregion: log-test
