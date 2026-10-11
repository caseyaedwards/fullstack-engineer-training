package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"fsp/examples/week12/your-monitor/internal/auth"
)

// region: users

// AuthStore implements auth.Store on the users and sessions tables.
type AuthStore struct{ pool *pgxpool.Pool }

func NewAuthStore(pool *pgxpool.Pool) *AuthStore { return &AuthStore{pool: pool} }

func (s *AuthStore) CreateUser(ctx context.Context, email, passwordHash string) (auth.User, error) {
	u := auth.User{Email: email}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id, created_at`,
		email, passwordHash).Scan(&u.ID, &u.CreatedAt)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation on email
		return auth.User{}, auth.ErrEmailTaken
	}
	return u, err
}

func (s *AuthStore) UserByEmail(ctx context.Context, email string) (auth.User, string, error) {
	var u auth.User
	var hash string
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, created_at, password_hash FROM users WHERE email = $1`,
		email).Scan(&u.ID, &u.Email, &u.CreatedAt, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.User{}, "", auth.ErrNoUser
	}
	return u, hash, err
}

// endregion: users

// region: sessions

func (s *AuthStore) CreateSession(ctx context.Context, id string, userID int64, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO sessions (id, user_id, expires_at) VALUES ($1, $2, $3)`, id, userID, expiresAt)
	return err
}

// SessionUser joins the session to its user in one query. The expiry
// check runs in the WHERE clause: an expired row is never a match.
func (s *AuthStore) SessionUser(ctx context.Context, id string, now time.Time) (auth.User, error) {
	var u auth.User
	err := s.pool.QueryRow(ctx, `
		SELECT u.id, u.email, u.created_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.id = $1 AND s.expires_at > $2`, id, now).Scan(&u.ID, &u.Email, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.User{}, auth.ErrNoSession
	}
	return u, err
}

func (s *AuthStore) DeleteSession(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, id)
	return err
}

func (s *AuthStore) DeleteExpiredSessions(ctx context.Context, now time.Time) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at <= $1`, now)
	return err
}

// endregion: sessions
