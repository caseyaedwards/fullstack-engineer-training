package auth_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"fsp/examples/week13/your-monitor/internal/auth"
	"fsp/examples/week13/your-monitor/internal/db"
	"fsp/examples/week13/your-monitor/internal/db/dbtest"
)

// newService returns a Service over a fresh schema, with one user and a
// clock that the test moves by hand.
func newService(t *testing.T) (*auth.Service, *time.Time) {
	t.Helper()
	pool := dbtest.Pool(t, dbtest.URL(t), db.Migrate)
	svc := auth.NewService(db.NewAuthStore(pool))
	svc.Cost = bcrypt.MinCost // cost 12 takes about 0.25 s per hash, far longer under -race
	now := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	svc.Now = func() time.Time { return now }
	if _, err := svc.CreateUser(context.Background(), " You@Example.com ", "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	return svc, &now
}

// region: service-test

func TestLoginLogout(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()

	sess, err := svc.Login(ctx, "YOU@example.com", "correct horse battery")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if sess.User.Email != "you@example.com" || len(sess.Token) < 26 {
		t.Errorf("session %+v: want the lowercased email and a token of 26 or more characters", sess)
	}

	// The table holds the SHA-256 of the token, never the token itself.
	sum := sha256.Sum256([]byte(sess.Token))
	id := hex.EncodeToString(sum[:])
	if u, err := svc.User(ctx, sess.Token); err != nil || u.ID != sess.User.ID {
		t.Fatalf("User(token) = %+v, %v", u, err)
	}
	if _, err := svc.Store.SessionUser(ctx, sess.Token, time.Now()); !errors.Is(err, auth.ErrNoSession) {
		t.Errorf("the raw token finds a session row: %v", err)
	}
	if _, err := svc.Store.SessionUser(ctx, id, time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)); err != nil {
		t.Errorf("the SHA-256 of the token finds no session row: %v", err)
	}

	if err := svc.Logout(ctx, sess.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.User(ctx, sess.Token); !errors.Is(err, auth.ErrNoSession) {
		t.Errorf("after logout: err = %v, want ErrNoSession", err)
	}
}

func TestBadCredentialsLookTheSame(t *testing.T) {
	svc, _ := newService(t)
	for _, c := range []struct{ email, password string }{
		{"you@example.com", "wrong horse battery"},
		{"nobody@example.com", "correct horse battery"},
		{"you@example.com", strings.Repeat("a", 73)},
	} {
		if _, err := svc.Login(context.Background(), c.email, c.password); !errors.Is(err, auth.ErrBadCredentials) {
			t.Errorf("Login(%q, %q): err = %v, want ErrBadCredentials", c.email, c.password, err)
		}
	}
}

// TestUnknownEmailCostsAFullCheck watches the password check. A login
// for an email with no account still runs one, against a hash at the
// production cost, or its speed gives the missing account away.
func TestUnknownEmailCostsAFullCheck(t *testing.T) {
	svc, _ := newService(t)
	var hashes []string
	svc.Check = func(hash, password string) bool {
		hashes = append(hashes, hash)
		return auth.CheckPassword(hash, password)
	}
	svc.Login(context.Background(), "nobody@example.com", "correct horse battery")
	if len(hashes) != 1 {
		t.Fatalf("%d password checks for an unknown email, want 1", len(hashes))
	}
	if cost, err := bcrypt.Cost([]byte(hashes[0])); err != nil || cost != auth.Cost {
		t.Errorf("checked against a hash of cost %d (err %v), want %d", cost, err, auth.Cost)
	}
}

// TestGoodLoginResetsTheLimit makes sure a user who mistypes now and then
// never builds up to a lockout: a good login clears the count.
func TestGoodLoginResetsTheLimit(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	login := func(password string) error {
		_, err := svc.Login(ctx, "you@example.com", password)
		return err
	}
	for i := range auth.MaxAttempts - 1 {
		if err := login("wrong horse battery"); !errors.Is(err, auth.ErrBadCredentials) {
			t.Fatalf("wrong password %d: err = %v", i+1, err)
		}
	}
	if err := login("correct horse battery"); err != nil {
		t.Fatalf("good login: %v", err)
	}
	for i := range auth.MaxAttempts {
		if err := login("wrong horse battery"); !errors.Is(err, auth.ErrBadCredentials) {
			t.Fatalf("wrong password %d after the good login: err = %v, want ErrBadCredentials", i+1, err)
		}
	}
}

func TestSessionExpires(t *testing.T) {
	svc, now := newService(t)
	sess, err := svc.Login(context.Background(), "you@example.com", "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	*now = now.Add(auth.SessionTTL - time.Second)
	if _, err := svc.User(context.Background(), sess.Token); err != nil {
		t.Errorf("1 s before expiry: %v", err)
	}
	*now = now.Add(time.Second)
	if _, err := svc.User(context.Background(), sess.Token); !errors.Is(err, auth.ErrNoSession) {
		t.Errorf("at expiry: err = %v, want ErrNoSession", err)
	}
}

func TestLoginIsRateLimitedPerEmail(t *testing.T) {
	svc, now := newService(t)
	ctx := context.Background()
	for range auth.MaxAttempts {
		svc.Login(ctx, "you@example.com", "wrong horse battery")
	}
	// The right password does not help once the attempts are used up.
	_, err := svc.Login(ctx, "you@example.com", "correct horse battery")
	var tooMany *auth.TooManyAttemptsError
	if !errors.As(err, &tooMany) || tooMany.Seconds() != 900 {
		t.Fatalf("attempt 6: err = %v, want TooManyAttemptsError with 900 seconds", err)
	}
	*now = now.Add(auth.AttemptWindow)
	if _, err := svc.Login(ctx, "you@example.com", "correct horse battery"); err != nil {
		t.Errorf("after the window: %v", err)
	}
}

// endregion: service-test

func TestCreateUserErrors(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	if _, err := svc.CreateUser(ctx, "you@EXAMPLE.com", "another long password"); !errors.Is(err, auth.ErrEmailTaken) {
		t.Errorf("same email in other case: err = %v, want ErrEmailTaken", err)
	}
	if _, err := svc.CreateUser(ctx, "not an email", "another long password"); err == nil {
		t.Error("an address without @ is accepted")
	}
	if _, err := svc.CreateUser(ctx, strings.Repeat("a", 243)+"@example.com", "another long password"); err == nil {
		t.Error("an address of 255 bytes is accepted")
	}
	if _, err := svc.CreateUser(ctx, "new@example.com", "short"); !errors.Is(err, auth.ErrPasswordTooShort) {
		t.Errorf("short password: err = %v", err)
	}
}

// region: log-test

// TestLoginLogNamesTheReasonNeverThePassword checks the log lines that
// Observe It reads.
func TestLoginLogNamesTheReasonNeverThePassword(t *testing.T) {
	svc, _ := newService(t)
	var logs bytes.Buffer
	prev := slog.Default()
	// ReplaceAttr drops the time, which changes on every run.
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	ctx := context.Background()
	svc.Login(ctx, "you@example.com", "wrong horse battery")
	svc.Login(ctx, "nobody@example.com", "correct horse battery")
	svc.Login(ctx, "you@example.com", "correct horse battery")
	svc.Login(ctx, "evil\nlogin ok", "x")
	svc.Login(ctx, strings.Repeat("a", 300)+"@example.com", "x")

	want := `{"level":"WARN","msg":"login failed","email":"you@example.com","reason":"wrong password"}
{"level":"WARN","msg":"login failed","email":"nobody@example.com","reason":"no such user"}
{"level":"INFO","msg":"login ok","user_id":1,"email":"you@example.com"}
{"level":"WARN","msg":"login failed","email":"evil\nlogin ok","reason":"no such user"}
{"level":"WARN","msg":"login failed","email_bytes":312,"reason":"email too long"}
`
	if logs.String() != want {
		t.Errorf("log:\n%s\nwant:\n%s", logs.String(), want)
	}
	if strings.Contains(logs.String(), "horse") {
		t.Error("the log holds a password")
	}
}

// endregion: log-test
