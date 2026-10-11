package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"time"
)

// region: service

// SessionTTL is how long a login lasts. The cookie and the sessions row
// both end then.
const SessionTTL = 24 * time.Hour

// Login attempts allowed per email address in each window.
const (
	MaxAttempts   = 5
	AttemptWindow = 15 * time.Minute
)

// Service logs users in and out. Create it with NewService. Tests change
// Cost and Now before the first call.
type Service struct {
	Store Store
	Cost  int              // bcrypt cost of new hashes
	Now   func() time.Time // the clock; tests set a fixed one

	limiter *limiter
}

func NewService(store Store) *Service {
	return &Service{Store: store, Cost: Cost, Now: time.Now, limiter: newLimiter(MaxAttempts, AttemptWindow)}
}

// TooManyAttemptsError is the error Login returns once an email address
// has used up its attempts. RetryAfter says when the next one is allowed.
type TooManyAttemptsError struct{ RetryAfter time.Duration }

func (e *TooManyAttemptsError) Error() string {
	return fmt.Sprintf("too many login attempts; try again in %d seconds", e.Seconds())
}

// Seconds is RetryAfter in whole seconds, rounded up: the value of the
// Retry-After header.
func (e *TooManyAttemptsError) Seconds() int { return seconds(e.RetryAfter) }

// seconds rounds d up to whole seconds, at least 1.
func seconds(d time.Duration) int {
	return max(1, int((d+time.Second-1)/time.Second))
}

// endregion: service

// region: create

// CreateUser adds a user with a bcrypt hash of password. The email is
// stored in lowercase, and login lowercases what people type.
func (s *Service) CreateUser(ctx context.Context, email, password string) (User, error) {
	email = normalize(email)
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email {
		return User{}, fmt.Errorf("%q is not an email address", email)
	}
	hash, err := HashPassword(password, s.Cost)
	if err != nil {
		return User{}, err
	}
	return s.Store.CreateUser(ctx, email, hash)
}

func normalize(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

// endregion: create

// region: login

// Login checks the email and password and starts a new session. A wrong
// email and a wrong password return the same ErrBadCredentials, after the
// same bcrypt work: neither the answer nor its timing tells an attacker
// which emails have an account. The log tells you.
func (s *Service) Login(ctx context.Context, email, password string) (Session, error) {
	email = normalize(email)
	if ok, wait := s.limiter.take(email, s.Now()); !ok {
		slog.WarnContext(ctx, "login refused", "email", email, "reason", "too many attempts")
		return Session{}, &TooManyAttemptsError{RetryAfter: wait}
	}

	user, hash, err := s.Store.UserByEmail(ctx, email)
	if errors.Is(err, ErrNoUser) {
		CheckPassword(dummyHash, password) // spend the same time as a real check
		slog.WarnContext(ctx, "login failed", "email", email, "reason", "no such user")
		return Session{}, ErrBadCredentials
	}
	if err != nil {
		return Session{}, err
	}
	if !CheckPassword(hash, password) {
		slog.WarnContext(ctx, "login failed", "email", email, "reason", "wrong password")
		return Session{}, ErrBadCredentials
	}
	s.limiter.reset(email)

	// A new random token for every login. Reusing a token from before the
	// login lets whoever planted it ride along: session fixation.
	now := s.Now()
	sess := Session{Token: rand.Text(), User: user, ExpiresAt: now.Add(SessionTTL)}
	if err := s.Store.DeleteExpiredSessions(ctx, now); err != nil {
		return Session{}, err
	}
	if err := s.Store.CreateSession(ctx, tokenID(sess.Token), user.ID, sess.ExpiresAt); err != nil {
		return Session{}, err
	}
	slog.InfoContext(ctx, "login ok", "user_id", user.ID, "email", email)
	return sess, nil
}

// dummyHash is a cost-12 hash of a random password that nobody knows.
// Checking a password against it takes as long as a real check, because
// bcrypt reads the cost from the hash. TestDummyHashCost keeps it in step
// with Cost.
const dummyHash = "$2a$12$zALevm5pKefuVcifuimGveOiwHvDYHhYr3QadgPP0l0LuYyObfNu2"

// endregion: login

// region: session

// tokenID is the sessions.id of a token: its SHA-256 hash in hex. A fast
// hash is enough here, unlike for passwords: the token holds 128 random
// bits, which no one can guess at any speed.
func tokenID(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// User returns the user of the session that token names, or ErrNoSession.
func (s *Service) User(ctx context.Context, token string) (User, error) {
	if token == "" {
		return User{}, ErrNoSession
	}
	return s.Store.SessionUser(ctx, tokenID(token), s.Now())
}

// Logout ends the session that token names. Ending a session that does
// not exist is not an error: the result is the same.
func (s *Service) Logout(ctx context.Context, token string) error {
	return s.Store.DeleteSession(ctx, tokenID(token))
}

// endregion: session
