// contract stub: week 08

package monitor

import "time"

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
