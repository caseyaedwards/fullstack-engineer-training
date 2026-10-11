package main

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/crypto/bcrypt"

	"fsp/examples/week16/your-monitor/internal/api"
	"fsp/examples/week16/your-monitor/internal/auth"
	"fsp/examples/week16/your-monitor/internal/db"
	"fsp/examples/week16/your-monitor/internal/db/dbtest"
)

// region: create-user-test

func TestCreateUserReadsThePasswordFromStdin(t *testing.T) {
	tests := []struct {
		name, stdin, wantOut, wantErr string
	}{
		{"line from printf", "correct horse battery\n", "created user id=1 email=you@example.com\n", ""},
		{"Windows line end", "correct horse battery\r\n", "created user id=1 email=you@example.com\n", ""},
		{"no line end", "correct horse battery", "created user id=1 email=you@example.com\n", ""},
		{"empty stdin", "", "", "no password on stdin"},
		{"short password", "hunter2\n", "", "password must have at least 15 characters"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			users := auth.NewService(db.NewAuthStore(dbtest.Pool(t, dbtest.URL(t), db.Migrate)))
			users.Cost = bcrypt.MinCost
			var out strings.Builder
			err := createUser(context.Background(), users, "you@example.com", strings.NewReader(tt.stdin), &out)
			if got := errString(err); got != tt.wantErr || out.String() != tt.wantOut {
				t.Fatalf("got %q, error %q; want %q, error %q", out.String(), got, tt.wantOut, tt.wantErr)
			}
			if tt.wantErr == "" {
				if _, err := users.Login(context.Background(), "you@example.com", "correct horse battery"); err != nil {
					t.Errorf("login with the password from stdin: %v", err)
				}
			}
		})
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// endregion: create-user-test

// region: flow-test

// TestLoginFlowInABrowser runs the api over HTTPS with Postgres behind it,
// and a client that keeps cookies the way a browser does.
func TestLoginFlowInABrowser(t *testing.T) {
	pool := dbtest.Pool(t, dbtest.URL(t), db.Migrate)
	users := auth.NewService(db.NewAuthStore(pool))
	users.Cost = bcrypt.MinCost
	if _, err := users.CreateUser(context.Background(), "you@example.com", "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	h := newHandler(pool, users, api.NewMetrics(prometheus.NewRegistry()))
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	browser := srv.Client() // trusts the test certificate
	browser.Jar, _ = cookiejar.New(nil)

	step := func(method, path, body string, want int) {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := browser.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Fatalf("%s %s: status %d, want %d", method, path, resp.StatusCode, want)
		}
	}
	const monitor = `{"url":"https://example.com","interval_seconds":60}`
	step("POST", "/api/monitors", monitor, 401)
	step("GET", "/api/me", "", 401)
	step("POST", "/api/login", `{"email":"you@example.com","password":"correct horse battery"}`, 200)
	step("GET", "/api/me", "", 200)
	step("POST", "/api/monitors", monitor, 201)
	step("GET", "/api/monitors", "", 200)

	// The same browser on plain HTTP: the jar keeps the Secure cookie to
	// itself, and the api sees no session.
	plain := httptest.NewServer(h)
	t.Cleanup(plain.Close)
	req, _ := http.NewRequest("GET", plain.URL+"/api/me", nil)
	resp, err := browser.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("GET /api/me over plain HTTP: status %d, want 401", resp.StatusCode)
	}

	step("POST", "/api/logout", "", 204)
	step("GET", "/api/me", "", 401)
	step("DELETE", "/api/monitors/1", "", 401)
}

// endregion: flow-test
