// contract stub: week 08

package monitor

import (
	"context"
	"time"
)

// Result is one row of check_results. Zero values stand for NULL.
type Result struct {
	MonitorID   int64     `json:"monitor_id"`
	CheckedAt   time.Time `json:"checked_at"`
	Up          bool      `json:"up"`
	StatusCode  int       `json:"status_code"`
	DNSMs       int       `json:"dns_ms"`
	ConnectMs   int       `json:"connect_ms"`
	TLSMs       int       `json:"tls_ms"`
	FirstByteMs int       `json:"first_byte_ms"`
	TotalMs     int       `json:"total_ms"`
	Error       string    `json:"error"`
}

// ResultStore keeps check results. db.ResultStore implements it.
type ResultStore interface {
	Insert(ctx context.Context, r Result) error
	// Recent returns up to limit results of one monitor, newest first.
	Recent(ctx context.Context, monitorID int64, limit int) ([]Result, error)
}
