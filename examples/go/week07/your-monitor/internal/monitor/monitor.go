// Package monitor holds what the uptime monitor knows about one monitored
// URL: the Monitor type, the rules a new monitor must follow, and the Store
// interface that keeps monitors. Week 7 keeps them in memory with MemStore;
// Week 8 adds a Postgres Store behind the same interface.
package monitor

// region: types

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"
)

// Monitor is one URL that the worker checks every IntervalSeconds. The
// struct tags name the JSON fields the api reads and writes.
type Monitor struct {
	ID              int64     `json:"id"`
	URL             string    `json:"url"`
	IntervalSeconds int       `json:"interval_seconds"`
	CreatedAt       time.Time `json:"created_at"`
}

// ErrNotFound and ErrDuplicate are the errors a Store returns for a
// missing monitor and for a URL it already holds. Callers compare with
// errors.Is, which also matches an error that wraps one of them.
var (
	ErrNotFound  = errors.New("monitor not found")
	ErrDuplicate = errors.New("monitor already exists")
)

// Store keeps monitors. Every method takes a context, because the Postgres
// Store in Week 8 sends a query that the context can cancel.
type Store interface {
	List(ctx context.Context) ([]Monitor, error)
	Get(ctx context.Context, id int64) (Monitor, error)
	Create(ctx context.Context, m Monitor) (Monitor, error) // sets ID and CreatedAt
	Delete(ctx context.Context, id int64) error
}

// endregion: types

// region: validate

// Interval limits, in seconds. A check every 30 seconds is the fastest the
// worker runs; once an hour is the slowest that still catches an outage.
const (
	MinIntervalSeconds = 30
	MaxIntervalSeconds = 3600
)

// Validate returns an error that names the first rule rawURL or
// intervalSeconds breaks, or nil when both are valid. The error text is
// written for the person who sent the request.
func Validate(rawURL string, intervalSeconds int) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("url is not a valid URL: %q", rawURL)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("url must start with http:// or https://: %q", rawURL)
	}
	if u.Host == "" {
		return fmt.Errorf("url must name a host: %q", rawURL)
	}
	if intervalSeconds < MinIntervalSeconds || intervalSeconds > MaxIntervalSeconds {
		return fmt.Errorf("interval_seconds must be between %d and %d, got %d",
			MinIntervalSeconds, MaxIntervalSeconds, intervalSeconds)
	}
	return nil
}

// endregion: validate
