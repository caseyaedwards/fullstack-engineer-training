// Package systemdesign retries a failing call with exponential backoff and
// full jitter, inside the time budget of a context.
package systemdesign

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"time"
)

// region: permanent

// permanentError marks an error that a retry cannot fix.
type permanentError struct{ err error }

func (p *permanentError) Error() string { return p.err.Error() }
func (p *permanentError) Unwrap() error { return p.err }

// Permanent wraps err to tell Retry to stop: a 404 or a validation error
// fails the same way on every attempt. Permanent(nil) returns nil.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &permanentError{err: err}
}

// endregion: permanent

// region: retry

// sleep waits for d, or until ctx is done. Tests replace it.
var sleep = func(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Retry calls op up to attempts times, until it returns nil or a Permanent
// error, or until ctx is done. Between two calls, it sleeps for a random
// time between zero and a ceiling that starts at base, doubles after each
// failure, and never exceeds max. An attempts below 1 counts as 1.
func Retry(ctx context.Context, attempts int, base, max time.Duration, op func(context.Context) error) error {
	if attempts < 1 {
		attempts = 1
	}
	var err error
	for i := range attempts {
		if i > 0 {
			if serr := sleep(ctx, backoff(i-1, base, max)); serr != nil {
				return serr
			}
		}
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		err = op(ctx)
		if err == nil {
			return nil
		}
		var p *permanentError
		if errors.As(err, &p) {
			return p.err
		}
	}
	return fmt.Errorf("after %d attempts: %w", attempts, err)
}

// backoff returns the full-jitter wait before retry number n, counted
// from 0: a random duration between 0 and min(max, base * 2^n).
func backoff(n int, base, max time.Duration) time.Duration {
	ceiling := base
	for range n {
		ceiling *= 2
		if ceiling >= max || ceiling <= 0 { // <= 0: the doubling overflowed
			ceiling = max
			break
		}
	}
	if ceiling > max {
		ceiling = max
	}
	if ceiling <= 0 {
		return 0
	}
	if ceiling == math.MaxInt64 { // ceiling + 1 overflows
		return rand.N(ceiling)
	}
	return rand.N(ceiling + 1)
}

// endregion: retry
