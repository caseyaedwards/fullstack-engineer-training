package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"fsp/examples/week09/your-monitor/internal/monitor"
)

// region: monitorstore

// MonitorStore is a monitor.Store backed by the monitors table.
type MonitorStore struct {
	pool *pgxpool.Pool
}

// NewMonitorStore returns a MonitorStore that queries through pool.
func NewMonitorStore(pool *pgxpool.Pool) *MonitorStore {
	return &MonitorStore{pool: pool}
}

// uniqueViolation is the Postgres error code (SQLSTATE) for a row that
// breaks a unique constraint.
const uniqueViolation = "23505"

// Create inserts m and returns it with the ID and CreatedAt that
// Postgres assigned.
func (s *MonitorStore) Create(ctx context.Context, m monitor.Monitor) (monitor.Monitor, error) {
	err := s.pool.QueryRow(ctx,
		`insert into monitors (url, interval_seconds) values ($1, $2)
		 returning id, created_at`,
		m.URL, m.IntervalSeconds,
	).Scan(&m.ID, &m.CreatedAt)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return monitor.Monitor{}, monitor.ErrDuplicate
	}
	if err != nil {
		return monitor.Monitor{}, err
	}
	return m, nil
}

// endregion: monitorstore

// region: get

// Get returns the monitor with this id, or monitor.ErrNotFound.
func (s *MonitorStore) Get(ctx context.Context, id int64) (monitor.Monitor, error) {
	var m monitor.Monitor
	err := s.pool.QueryRow(ctx,
		`select id, url, interval_seconds, created_at from monitors where id = $1`, id,
	).Scan(&m.ID, &m.URL, &m.IntervalSeconds, &m.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return monitor.Monitor{}, monitor.ErrNotFound
	}
	return m, err
}

// List returns every monitor, oldest first.
func (s *MonitorStore) List(ctx context.Context) ([]monitor.Monitor, error) {
	rows, err := s.pool.Query(ctx,
		`select id, url, interval_seconds, created_at from monitors order by id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ms := []monitor.Monitor{}
	for rows.Next() {
		var m monitor.Monitor
		if err := rows.Scan(&m.ID, &m.URL, &m.IntervalSeconds, &m.CreatedAt); err != nil {
			return nil, err
		}
		ms = append(ms, m)
	}
	return ms, rows.Err()
}

// Delete removes the monitor with this id, and through on delete
// cascade, its results. It returns monitor.ErrNotFound when no row
// matches.
func (s *MonitorStore) Delete(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `delete from monitors where id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return monitor.ErrNotFound
	}
	return nil
}

// endregion: get
