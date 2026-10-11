package api

import (
	"bytes"
	"log"
	"regexp"
	"testing"

	"fsp/examples/week09/your-monitor/internal/monitor"
)

// region: log-test

func TestLogRequests(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0) // no date and time: the test compares the line
	h := LogRequests(logger, NewHandler(monitor.NewMemStore(), &spyResults{}, nil))

	serve(h, "POST", "/api/monitors", "application/json", `{"url":"https://example.com","interval_seconds":60}`)
	serve(h, "GET", "/api/monitors/7", "", "")
	serve(h, "DELETE", "/api/monitors/1", "", "")

	want := regexp.MustCompile(`^request method=POST path=/api/monitors status=201 bytes=\d+ duration_ms=\d+\.\d
request method=GET path=/api/monitors/7 status=404 bytes=32 duration_ms=\d+\.\d
request method=DELETE path=/api/monitors/1 status=204 bytes=0 duration_ms=\d+\.\d
$`)
	if !want.Match(buf.Bytes()) {
		t.Errorf("log lines:\n%s\ndo not match:\n%s", buf.String(), want)
	}
}

// endregion: log-test
