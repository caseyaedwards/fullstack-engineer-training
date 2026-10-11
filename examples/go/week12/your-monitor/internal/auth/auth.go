// Package auth logs people in and out of the uptime monitor. It hashes
// passwords with bcrypt, keeps one row per login in the sessions table, and
// limits how fast anyone can guess a password.
package auth

import (
	"context"
	"errors"
	"time"
)

// region: types

// User is one row of users, without the password hash. The hash never
// leaves this package and the store: no JSON field can leak it.
type User struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// Session is one login. Token goes into the cookie and nowhere else: the
// sessions table stores only its SHA-256 hash.
type Session struct {
	Token     string
	User      User
	ExpiresAt time.Time
}

var (
	ErrBadCredentials = errors.New("wrong email or password")
	ErrNoSession      = errors.New("no session, or the session expired")
	ErrNoUser         = errors.New("no user with this email")
	ErrEmailTaken     = errors.New("a user with this email exists")
)

// Store keeps users and sessions. db.AuthStore implements it on Postgres.
type Store interface {
	CreateUser(ctx context.Context, email, passwordHash string) (User, error)
	// UserByEmail returns the user and the password hash, or ErrNoUser.
	UserByEmail(ctx context.Context, email string) (User, string, error)
	CreateSession(ctx context.Context, id string, userID int64, expiresAt time.Time) error
	// SessionUser returns the user of a session that expires after now,
	// or ErrNoSession.
	SessionUser(ctx context.Context, id string, now time.Time) (User, error)
	DeleteSession(ctx context.Context, id string) error
	DeleteExpiredSessions(ctx context.Context, now time.Time) error
}

// endregion: types

// region: context

type userKey struct{}

// WithUser returns a copy of ctx that carries u. The api adds the user of
// the session this way, for the handlers behind the login check.
func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, userKey{}, u)
}

// UserFrom returns the user that WithUser added, and false when there is
// none.
func UserFrom(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(userKey{}).(User)
	return u, ok
}

// endregion: context
