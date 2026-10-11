// contract stub: week 08
//
// Package db applies the migrations and implements the stores on Postgres.
// This file is the smallest version of the Week 8 contract that Week 9
// needs.
package db

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"fsp/examples/week18/your-monitor/internal/monitor"
	"fsp/examples/week18/your-monitor/migrations"
)

// Migrate applies every migration in migrations.FS that schema_migrations
// does not list yet, each in its own transaction, in version order. An
// advisory lock keeps two programs that start together from both applying
// the same file.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(8008)`); err != nil {
		return err
	}
	defer func() {
		// The lock belongs to this connection. If the unlock fails, close the
		// connection, which ends the lock, instead of handing a locked
		// connection back to the pool, where it blocks the Migrate of every
		// other program.
		ctx := context.WithoutCancel(ctx)
		if _, err := conn.Exec(ctx, `SELECT pg_advisory_unlock(8008)`); err != nil {
			conn.Conn().Close(ctx)
		}
	}()

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version int PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	names, err := fs.Glob(migrations.FS, "*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		version, err := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
		if err != nil {
			return fmt.Errorf("migration %s: no version number", name)
		}
		var done bool
		if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&done); err != nil {
			return err
		}
		if done {
			continue
		}
		sql, err := fs.ReadFile(migrations.FS, name)
		if err != nil {
			return err
		}
		err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, string(sql)); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version)
			return err
		})
		if err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
	}
	return nil
}

// MonitorStore implements monitor.Store on the monitors table.
type MonitorStore struct{ pool *pgxpool.Pool }

func NewMonitorStore(pool *pgxpool.Pool) *MonitorStore { return &MonitorStore{pool: pool} }

func (s *MonitorStore) List(ctx context.Context) ([]monitor.Monitor, error) {
	rows, _ := s.pool.Query(ctx, `SELECT id, url, interval_seconds, created_at FROM monitors ORDER BY id`)
	ms, err := pgx.CollectRows(rows, pgx.RowToStructByPos[monitor.Monitor])
	if ms == nil {
		ms = []monitor.Monitor{}
	}
	return ms, err
}

func (s *MonitorStore) Get(ctx context.Context, id int64) (monitor.Monitor, error) {
	var m monitor.Monitor
	err := s.pool.QueryRow(ctx, `SELECT id, url, interval_seconds, created_at FROM monitors WHERE id = $1`, id).
		Scan(&m.ID, &m.URL, &m.IntervalSeconds, &m.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, monitor.ErrNotFound
	}
	return m, err
}

func (s *MonitorStore) Create(ctx context.Context, m monitor.Monitor) (monitor.Monitor, error) {
	err := s.pool.QueryRow(ctx, `INSERT INTO monitors (url, interval_seconds) VALUES ($1, $2) RETURNING id, created_at`,
		m.URL, m.IntervalSeconds).Scan(&m.ID, &m.CreatedAt)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return m, monitor.ErrDuplicate
	}
	return m, err
}

func (s *MonitorStore) Delete(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM monitors WHERE id = $1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return monitor.ErrNotFound
	}
	return err
}

// ResultStore reads and writes check_results.
type ResultStore struct{ pool *pgxpool.Pool }

func NewResultStore(pool *pgxpool.Pool) *ResultStore { return &ResultStore{pool: pool} }

func (s *ResultStore) Insert(ctx context.Context, r monitor.Result) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO check_results
		(monitor_id, checked_at, up, status_code, dns_ms, connect_ms, tls_ms, first_byte_ms, total_ms, error)
		VALUES ($1, $2, $3, NULLIF($4, 0), $5, $6, $7, $8, $9, NULLIF($10, ''))`,
		r.MonitorID, r.CheckedAt, r.Up, r.StatusCode, r.DNSMs, r.ConnectMs, r.TLSMs, r.FirstByteMs, r.TotalMs, r.Error)
	return err
}

// Recent returns up to limit results of one monitor, newest first.
func (s *ResultStore) Recent(ctx context.Context, monitorID int64, limit int) ([]monitor.Result, error) {
	rows, _ := s.pool.Query(ctx, `SELECT monitor_id, checked_at, up, coalesce(status_code, 0),
		coalesce(dns_ms, 0), coalesce(connect_ms, 0), coalesce(tls_ms, 0), coalesce(first_byte_ms, 0),
		coalesce(total_ms, 0), coalesce(error, '')
		FROM check_results WHERE monitor_id = $1 ORDER BY checked_at DESC LIMIT $2`, monitorID, limit)
	rs, err := pgx.CollectRows(rows, pgx.RowToStructByPos[monitor.Result])
	if rs == nil {
		rs = []monitor.Result{}
	}
	return rs, err
}
