package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// region: webhook-test

func TestWebhookPostsTheEventAsJSON(t *testing.T) {
	var got Event
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("got %s with Content-Type %q, want POST with application/json", r.Method, r.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
	}))
	defer srv.Close()

	want := Event{Kind: Opened, IncidentID: 7, MonitorID: 3, URL: "https://example.com",
		Cause: "status 503", At: time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)}
	if err := (Webhook{URL: srv.URL + "/hooks/secret-token"}).Notify(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("receiver got %+v, want %+v", got, want)
	}
}

func TestWebhookErrorsKeepTheSecretOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	defer srv.Close()
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close() // nothing listens on this address any more

	for _, base := range []string{srv.URL, closed.URL} {
		err := (Webhook{URL: base + "/hooks/secret-token"}).Notify(context.Background(), Event{Kind: Opened})
		if err == nil {
			t.Fatalf("%s: Notify returned no error", base)
		}
		if strings.Contains(err.Error(), "secret-token") {
			t.Errorf("error %q quotes the secret path of the webhook URL", err)
		}
	}
}

// endregion: webhook-test

func TestLogWritesOneLine(t *testing.T) {
	var buf bytes.Buffer
	l := Log{Logger: log.New(&buf, "", 0)}
	l.Notify(context.Background(), Event{Kind: Resolved, IncidentID: 2, MonitorID: 1, URL: "https://example.com", Cause: "status 500"})
	want := "alert resolved incident=2 monitor=1 url=https://example.com cause=\"status 500\"\n"
	if buf.String() != want {
		t.Errorf("log line %q, want %q", buf.String(), want)
	}
}
