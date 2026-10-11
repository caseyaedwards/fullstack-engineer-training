package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"fsp/examples/week13/your-monitor/internal/monitor"
)

// region: helpers

// newTestHandler returns the API over a fresh MemStore that holds one
// monitor: ID 1, https://example.com, every 60 seconds.
func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	store := monitor.NewMemStore()
	if _, err := store.Create(context.Background(), monitor.Monitor{URL: "https://example.com", IntervalSeconds: 60}); err != nil {
		t.Fatal(err)
	}
	return NewHandler(store, &spyResults{}, nil, fakeAuth{})
}

// serve sends one request to h and returns the recorded response. A
// non-empty contentType sets the Content-Type header. The request carries
// the session cookie of a logged-in user, which the writes need since
// Week 11.
func serve(h http.Handler, method, path, contentType, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.AddCookie(&http.Cookie{Name: "session", Value: goodToken})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// endregion: helpers

// region: healthz

func TestHealthz(t *testing.T) {
	tests := []struct {
		method, path string
		wantStatus   int
	}{
		{http.MethodGet, "/healthz", http.StatusOK},
		{http.MethodPost, "/healthz", http.StatusMethodNotAllowed},
		{http.MethodGet, "/monitors", http.StatusNotFound}, // the routes live under /api/
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			NewHandler(monitor.NewMemStore(), &spyResults{}, nil, fakeAuth{}).ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantStatus != http.StatusOK {
				return
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			var body struct{ Status string }
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("body is not JSON: %v", err)
			}
			if body.Status != "ok" {
				t.Errorf(`status field = %q, want "ok"`, body.Status)
			}
		})
	}
}

// endregion: healthz

// region: errors-test

// TestErrors sends one bad request per row and checks the status code and
// the exact {"error": ...} body.
func TestErrors(t *testing.T) {
	const jsonType = "application/json"
	tests := []struct {
		name, method, path, contentType, body string
		wantStatus                            int
		wantError                             string
	}{
		{"id absent", "GET", "/api/monitors/2", "", "", 404, "monitor 2 not found"},
		{"id not an integer", "GET", "/api/monitors/abc", "", "", 400, `id must be an integer, got "abc"`},
		{"delete absent", "DELETE", "/api/monitors/2", "", "", 404, "monitor 2 not found"},
		{"no Content-Type", "POST", "/api/monitors", "", `{"url":"https://example.org","interval_seconds":60}`, 415, "Content-Type must be application/json"},
		{"form body", "POST", "/api/monitors", "application/x-www-form-urlencoded", `url=https://example.org`, 415, "Content-Type must be application/json"},
		{"empty body", "POST", "/api/monitors", jsonType, "", 400, "body is empty"},
		{"not JSON", "POST", "/api/monitors", jsonType, `{"url": https://example.org}`, 400, "body is not valid JSON"},
		{"cut off", "POST", "/api/monitors", jsonType, `{"url": "https://example.org"`, 400, "body is not valid JSON"},
		{"array", "POST", "/api/monitors", jsonType, `[]`, 400, "body must be a JSON object, got a JSON array"},
		{"null", "POST", "/api/monitors", jsonType, `null`, 400, "body must be a JSON object, got a JSON null"},
		{"two objects", "POST", "/api/monitors", jsonType, `{"url":"https://example.org","interval_seconds":60} {}`, 400, "body must hold a single JSON object"},
		{"array after the object", "POST", "/api/monitors", jsonType, `{"url":"https://example.org","interval_seconds":60} []`, 400, "body must hold a single JSON object"},
		{"text after the object", "POST", "/api/monitors", jsonType, `{"url":"https://example.org","interval_seconds":60} x`, 400, "body is not valid JSON"},
		{"typo in a field", "POST", "/api/monitors", jsonType, `{"url":"https://example.org","interval":60}`, 400, `unknown field "interval"`},
		{"wrong type", "POST", "/api/monitors", jsonType, `{"url":"https://example.org","interval_seconds":"60"}`, 400, "interval_seconds has the wrong type: got a JSON string"},
		{"no scheme", "POST", "/api/monitors", jsonType, `{"url":"example.org","interval_seconds":60}`, 400, `url must start with http:// or https://: "example.org"`},
		{"interval missing", "POST", "/api/monitors", jsonType, `{"url":"https://example.org"}`, 400, "interval_seconds must be between 30 and 3600, got 0"},
		{"duplicate", "POST", "/api/monitors", jsonType, `{"url":"https://example.com","interval_seconds":30}`, 409, "a monitor for https://example.com already exists"},
		{"body over 1 MiB", "POST", "/api/monitors", jsonType, `{"url":"https://example.org/` + strings.Repeat("a", 1<<20) + `"}`, 413, "body is larger than 1048576 bytes"},
		{"unknown path", "GET", "/api/nothing", "", "", 404, "not found"},
		{"trailing slash", "GET", "/api/monitors/", "", "", 404, "not found"},
		{"wrong method", "PUT", "/api/monitors/1", jsonType, `{}`, 405, "method not allowed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serve(newTestHandler(t), tt.method, tt.path, tt.contentType, tt.body)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			var body struct{ Error string }
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("body %q is not JSON: %v", rec.Body, err)
			}
			if body.Error != tt.wantError {
				t.Errorf("error = %q, want %q", body.Error, tt.wantError)
			}
		})
	}
}

// TestCreateAccepts sends bodies that the API accepts, although they differ
// from the example in openapi.yaml.
func TestCreateAccepts(t *testing.T) {
	tests := []struct{ name, contentType, body string }{
		{"charset parameter", "application/json; charset=utf-8", `{"url":"https://example.org","interval_seconds":60}`},
		// encoding/json matches field names without regard to case.
		{"field names in another case", "application/json", `{"URL":"https://example.org","Interval_Seconds":60}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serve(newTestHandler(t), "POST", "/api/monitors", tt.contentType, tt.body)
			if rec.Code != http.StatusCreated {
				t.Errorf("status = %d, want 201; body %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestMethodNotAllowedListsAllowedMethods(t *testing.T) {
	rec := serve(newTestHandler(t), "PUT", "/api/monitors/1", "", "")
	if got, want := rec.Header().Get("Allow"), "DELETE, GET, HEAD"; got != want {
		t.Errorf("Allow = %q, want %q", got, want)
	}
}

// endregion: errors-test

// region: lifecycle-test

// TestMonitorLifecycle runs the API on a real port and walks one monitor
// through create, get, list and delete, as a client does.
func TestMonitorLifecycle(t *testing.T) {
	srv := httptest.NewServer(NewHandler(monitor.NewMemStore(), &spyResults{}, nil, fakeAuth{}))
	t.Cleanup(srv.Close)

	// The list starts empty, and empty means [], not null.
	resp := mustDo(t, "GET", srv.URL+"/api/monitors", "")
	if got := readBody(t, resp); got != "[]\n" {
		t.Errorf("empty list body = %q, want %q", got, "[]\n")
	}

	before := time.Now().Add(-time.Second)
	resp = mustDo(t, "POST", srv.URL+"/api/monitors", `{"url":"https://example.com","interval_seconds":60}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: status = %d, want 201; body %s", resp.StatusCode, readBody(t, resp))
	}
	if loc := resp.Header.Get("Location"); loc != "/api/monitors/1" {
		t.Errorf("Location = %q, want /api/monitors/1", loc)
	}
	var created monitor.Monitor
	decode(t, resp, &created)
	if created.ID != 1 || created.URL != "https://example.com" || created.IntervalSeconds != 60 || created.CreatedAt.Before(before) {
		t.Errorf("created = %+v, want ID 1, the URL and interval sent, and a CreatedAt from now", created)
	}

	var got monitor.Monitor
	decode(t, mustDo(t, "GET", srv.URL+resp.Header.Get("Location"), ""), &got)
	if got != created {
		t.Errorf("GET Location = %+v, want %+v", got, created)
	}

	var list []monitor.Monitor
	decode(t, mustDo(t, "GET", srv.URL+"/api/monitors", ""), &list)
	if len(list) != 1 || list[0] != created {
		t.Errorf("list = %+v, want [%+v]", list, created)
	}

	resp = mustDo(t, "DELETE", srv.URL+"/api/monitors/1", "")
	if body := readBody(t, resp); resp.StatusCode != http.StatusNoContent || body != "" {
		t.Errorf("delete: status %d, body %q; want 204 and no body", resp.StatusCode, body)
	}
	if resp := mustDo(t, "GET", srv.URL+"/api/monitors/1", ""); resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET after delete: status = %d, want 404", resp.StatusCode)
	}
}

// mustDo sends a request with an optional JSON body and returns the response.
func mustDo(t *testing.T, method, url, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.AddCookie(&http.Cookie{Name: "session", Value: goodToken}) // logged in
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func decode(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatal(err)
	}
}

// endregion: lifecycle-test

// region: fake-store-test

// brokenStore is a fake Store: every method returns err. With err nil,
// List returns a nil slice, which a Postgres store does for zero rows.
type brokenStore struct{ err error }

func (s brokenStore) List(context.Context) ([]monitor.Monitor, error) { return nil, s.err }
func (s brokenStore) Get(context.Context, int64) (monitor.Monitor, error) {
	return monitor.Monitor{}, s.err
}
func (s brokenStore) Create(context.Context, monitor.Monitor) (monitor.Monitor, error) {
	return monitor.Monitor{}, s.err
}
func (s brokenStore) Delete(context.Context, int64) error { return s.err }

func TestNilListIsAnEmptyArray(t *testing.T) {
	rec := serve(NewHandler(brokenStore{}, &spyResults{}, nil, fakeAuth{}), "GET", "/api/monitors", "", "")
	if rec.Code != http.StatusOK || rec.Body.String() != "[]\n" {
		t.Errorf("got %d %q, want 200 %q", rec.Code, rec.Body, "[]\n")
	}
}

// TestStoreFailureIsA500 checks that a store error answers 500 with a
// fixed message, and that only the log holds the error text.
func TestStoreFailureIsA500(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	h := NewHandler(brokenStore{err: errors.New("connection refused to db:5432")}, &spyResults{}, nil, fakeAuth{})
	requests := []struct{ method, path, body string }{
		{"GET", "/api/monitors", ""},
		{"GET", "/api/monitors/1", ""},
		{"POST", "/api/monitors", `{"url":"https://example.com","interval_seconds":60}`},
		{"DELETE", "/api/monitors/1", ""},
	}
	for _, r := range requests {
		rec := serve(h, r.method, r.path, "application/json", r.body)
		if rec.Code != http.StatusInternalServerError || rec.Body.String() != `{"error":"internal error"}`+"\n" {
			t.Errorf("%s %s: got %d %q, want 500 with internal error", r.method, r.path, rec.Code, rec.Body)
		}
		if strings.Contains(rec.Body.String(), "db:5432") {
			t.Errorf("%s %s: the response leaks the store error", r.method, r.path)
		}
	}
	if n := strings.Count(logs.String(), `err="connection refused to db:5432"`); n != len(requests) {
		t.Errorf("logged the store error %d times, want %d:\n%s", n, len(requests), logs.String())
	}
}

// endregion: fake-store-test
