package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"fsp/examples/week17/your-monitor/internal/auth"
	"fsp/examples/week17/your-monitor/internal/monitor"
)

// region: fake-auth

// goodToken is the one session token fakeAuth accepts.
const goodToken = "good-token"

var testUser = auth.User{ID: 1, Email: "you@example.com", CreatedAt: time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)}

// fakeAuth stands in for *auth.Service: one user, one password, one
// session, and one email that has used up its login attempts.
type fakeAuth struct{ loggedOut *string }

func (f fakeAuth) Login(ctx context.Context, email, password string) (auth.Session, error) {
	switch {
	case email == "locked@example.com":
		return auth.Session{}, &auth.TooManyAttemptsError{RetryAfter: 89*time.Second + time.Millisecond}
	case email == testUser.Email && password == "correct horse battery":
		return auth.Session{Token: goodToken, User: testUser}, nil
	}
	return auth.Session{}, auth.ErrBadCredentials
}

func (f fakeAuth) User(ctx context.Context, token string) (auth.User, error) {
	if token == goodToken {
		return testUser, nil
	}
	return auth.User{}, auth.ErrNoSession
}

func (f fakeAuth) Logout(ctx context.Context, token string) error {
	if f.loggedOut != nil {
		*f.loggedOut = token
	}
	return nil
}

// endregion: fake-auth

// region: require-test

// TestWritesNeedASession sends each request without a cookie and with a
// cookie that names no session. Reads answer as before; writes answer 401
// and change nothing.
func TestWritesNeedASession(t *testing.T) {
	tests := []struct {
		method, path, body string
		want               int
	}{
		{"GET", "/api/monitors", "", 200},
		{"GET", "/api/monitors/1", "", 200},
		{"POST", "/api/monitors", `{"url":"https://example.org","interval_seconds":60}`, 401},
		{"DELETE", "/api/monitors/1", "", 401},
		{"GET", "/api/me", "", 401},
	}
	for _, cookie := range []string{"", "session=forged"} {
		for _, tt := range tests {
			h := newTestHandler(t)
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			if cookie != "" {
				req.Header.Set("Cookie", cookie)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Errorf("%s %s with cookie %q: status %d, want %d", tt.method, tt.path, cookie, rec.Code, tt.want)
			}
			if tt.want == 401 && rec.Body.String() != `{"error":"login required"}`+"\n" {
				t.Errorf("%s %s: body %q", tt.method, tt.path, rec.Body)
			}
			// The store still holds exactly the one monitor it started with.
			var list []monitor.Monitor
			json.Unmarshal(serve(h, "GET", "/api/monitors", "", "").Body.Bytes(), &list)
			if len(list) != 1 || list[0].URL != "https://example.com" {
				t.Errorf("%s %s with cookie %q changed the store: %+v", tt.method, tt.path, cookie, list)
			}
		}
	}
}

// endregion: require-test

// region: login-test

func TestLoginSetsTheSessionCookie(t *testing.T) {
	rec := serve(newTestHandler(t), "POST", "/api/login", "application/json",
		`{"email":"you@example.com","password":"correct horse battery"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200; body %s", rec.Code, rec.Body)
	}
	want := "session=good-token; Path=/; Max-Age=86400; HttpOnly; Secure; SameSite=Lax"
	if got := rec.Header().Get("Set-Cookie"); got != want {
		t.Errorf("Set-Cookie:\n got %s\nwant %s", got, want)
	}
	if got := rec.Body.String(); got != `{"id":1,"email":"you@example.com","created_at":"2026-10-11T09:00:00Z"}`+"\n" {
		t.Errorf("body %s", got)
	}
}

func TestLoginErrors(t *testing.T) {
	tests := []struct {
		name, contentType, body string
		wantStatus              int
		wantError               string
	}{
		{"wrong password", "application/json", `{"email":"you@example.com","password":"wrong horse"}`, 401, "wrong email or password"},
		{"no such user", "application/json", `{"email":"who@example.com","password":"correct horse battery"}`, 401, "wrong email or password"},
		{"rate limited", "application/json", `{"email":"locked@example.com","password":"anything"}`, 429, "too many login attempts; try again in 90 seconds"},
		{"no password", "application/json", `{"email":"you@example.com"}`, 400, "email and password are required"},
		{"long email", "application/json", `{"email":"` + strings.Repeat("a", 243) + `@example.com","password":"x"}`, 400, "email must be at most 254 bytes"},
		{"form post", "application/x-www-form-urlencoded", "email=you@example.com&password=x", 415, "Content-Type must be application/json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serve(newTestHandler(t), "POST", "/api/login", tt.contentType, tt.body)
			var body struct{ Error string }
			json.Unmarshal(rec.Body.Bytes(), &body)
			if rec.Code != tt.wantStatus || body.Error != tt.wantError {
				t.Errorf("got %d %q, want %d %q", rec.Code, body.Error, tt.wantStatus, tt.wantError)
			}
			if c := rec.Header().Get("Set-Cookie"); c != "" {
				t.Errorf("a failed login sets a cookie: %s", c)
			}
			if tt.wantStatus == 429 && rec.Header().Get("Retry-After") != "90" {
				t.Errorf("Retry-After = %q, want 90", rec.Header().Get("Retry-After"))
			}
		})
	}
}

// endregion: login-test

// region: logout-test

func TestLogoutEndsTheSessionAndDeletesTheCookie(t *testing.T) {
	var loggedOut string
	h := NewHandler(monitor.NewMemStore(), nil, nil, fakeAuth{loggedOut: &loggedOut})
	rec := serve(h, "POST", "/api/logout", "", "") // serve sends the good cookie
	if rec.Code != http.StatusNoContent {
		t.Errorf("status %d, want 204", rec.Code)
	}
	if loggedOut != goodToken {
		t.Errorf("logged out token %q, want %q", loggedOut, goodToken)
	}
	want := "session=; Path=/; Max-Age=0; HttpOnly; Secure; SameSite=Lax"
	if got := rec.Header().Get("Set-Cookie"); got != want {
		t.Errorf("Set-Cookie:\n got %s\nwant %s", got, want)
	}
}

func TestMe(t *testing.T) {
	rec := serve(newTestHandler(t), "GET", "/api/me", "", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"email":"you@example.com"`) {
		t.Errorf("got %d %s, want 200 with the user", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "hash") {
		t.Errorf("the body names a hash: %s", rec.Body)
	}
}

// endregion: logout-test
