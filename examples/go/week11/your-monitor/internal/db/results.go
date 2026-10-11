package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"fsp/examples/week11/your-monitor/internal/monitor"
)

// region: resultstore

// ResultStore is a monitor.ResultStore backed by the check_results table.
type ResultStore struct {
	pool *pgxpool.Pool
}

// NewResultStore returns a ResultStore that queries through pool.
func NewResultStore(pool *pgxpool.Pool) *ResultStore {
	return &ResultStore{pool: pool}
}

// Insert adds one result. pgx sends a nil pointer as NULL.
func (s *ResultStore) Insert(ctx context.Context, r monitor.Result) error {
	_, err := s.pool.Exec(ctx,
		`insert into check_results (monitor_id, checked_at, up, status_code,
		     dns_ms, connect_ms, tls_ms, first_byte_ms, total_ms, error)
		 values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		r.MonitorID, r.CheckedAt, r.Up, r.StatusCode,
		r.DNSMs, r.ConnectMs, r.TLSMs, r.FirstByteMs, r.TotalMs, r.Error)
	return err
}

// recentSQL asks for exactly what the index check_results_monitor_time
// holds: one monitor, newest first, a limit. The index test explains this
// same query.
const recentSQL = `select monitor_id, checked_at, up, status_code,
       dns_ms, connect_ms, tls_ms, first_byte_ms, total_ms, error
  from check_results
 where monitor_id = $1
 order by checked_at desc
 limit $2`

// Recent returns up to limit results of one monitor, newest first.
func (s *ResultStore) Recent(ctx context.Context, monitorID int64, limit int) ([]monitor.Result, error) {
	rows, err := s.pool.Query(ctx, recentSQL, monitorID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := []monitor.Result{}
	for rows.Next() {
		var r monitor.Result
		err := rows.Scan(&r.MonitorID, &r.CheckedAt, &r.Up, &r.StatusCode,
			&r.DNSMs, &r.ConnectMs, &r.TLSMs, &r.FirstByteMs, &r.TotalMs, &r.Error)
		if err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// endregion: resultstore
