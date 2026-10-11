// Package check holds the result of checking one URL, and reads the list
// of URLs to check from a file. This is the package as Week 4 leaves it,
// plus Result.TimedOut from Week 5.
package check

import (
	"context"
	"errors"
	"fmt"
	"time"

	"fsp/examples/week06/your-monitor/internal/probe"
)

// Result is the outcome of checking one URL.
type Result struct {
	URL     string
	Status  int           // 0 when no response arrived
	Latency time.Duration // from sending the request to the headers arriving: Timings.FirstByte
	Err     error         // nil when a response arrived
	Timings probe.Timings
}

// Up reports whether a response arrived with a status code below 400.
func (r Result) Up() bool {
	return r.Err == nil && r.Status < 400
}

// String formats r as one line of key=value pairs.
func (r Result) String() string {
	ms := r.Latency.Milliseconds()
	switch {
	case r.Err != nil:
		return fmt.Sprintf("down url=%s latency_ms=%d error=%q", r.URL, ms, r.Err)
	case r.Status >= 400:
		return fmt.Sprintf("down url=%s status=%d latency_ms=%d", r.URL, r.Status, ms)
	default:
		return fmt.Sprintf("up url=%s status=%d latency_ms=%d", r.URL, r.Status, ms)
	}
}

// region: timedout

// TimedOut reports whether the check ran out of time: its context
// deadline passed before the response arrived in full.
func (r Result) TimedOut() bool {
	return errors.Is(r.Err, context.DeadlineExceeded)
}

// Cancelled reports whether the check stopped because its context was
// cancelled, as on SIGTERM, rather than because the site failed.
func (r Result) Cancelled() bool {
	return errors.Is(r.Err, context.Canceled)
}

// endregion: timedout
