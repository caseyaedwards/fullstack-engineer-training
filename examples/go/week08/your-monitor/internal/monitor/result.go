package monitor

import (
	"context"
	"time"
)

// region: result

// Result is one stored check of one monitor: a row of check_results.
// A nil pointer is NULL in the database and null in JSON. It means the
// value does not exist, which differs from zero: StatusCode is nil when
// no response arrived, TLSMs is nil for a plain http URL, DNSMs is nil
// when the URL holds an IP address. A DNS lookup that takes 0.4 ms is
// stored as 0, not as nil.
type Result struct {
	MonitorID   int64     `json:"monitor_id"`
	CheckedAt   time.Time `json:"checked_at"`
	Up          bool      `json:"up"`
	StatusCode  *int      `json:"status_code"`
	DNSMs       *int      `json:"dns_ms"`
	ConnectMs   *int      `json:"connect_ms"`
	TLSMs       *int      `json:"tls_ms"`
	FirstByteMs *int      `json:"first_byte_ms"`
	TotalMs     *int      `json:"total_ms"`
	Error       *string   `json:"error"`
}

// ResultStore keeps check results.
type ResultStore interface {
	Insert(ctx context.Context, r Result) error
	// Recent returns up to limit results of one monitor, newest first.
	Recent(ctx context.Context, monitorID int64, limit int) ([]Result, error)
}

// endregion: result
