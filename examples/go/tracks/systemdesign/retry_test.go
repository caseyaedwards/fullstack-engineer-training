package systemdesign

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeSleep replaces sleep for one test. It records each duration and
// returns at once, or returns the context error when ctx is done, as the
// real sleep does.
func fakeSleep(t *testing.T) *[]time.Duration {
	t.Helper()
	var slept []time.Duration
	real := sleep
	sleep = func(ctx context.Context, d time.Duration) error {
		slept = append(slept, d)
		return ctx.Err()
	}
	t.Cleanup(func() { sleep = real })
	return &slept
}

var errUnavailable = errors.New("503 Service Unavailable")

// region: tests

func TestRetrySucceedsOnThirdTry(t *testing.T) {
	slept := fakeSleep(t)
	calls := 0
	op := func(context.Context) error {
		calls++
		if calls < 3 {
			return errUnavailable
		}
		return nil
	}

	err := Retry(context.Background(), 5, 100*time.Millisecond, 150*time.Millisecond, op)
	if err != nil || calls != 3 {
		t.Fatalf("Retry = %v after %d calls, want nil after 3", err, calls)
	}
	if len(*slept) != 2 {
		t.Fatalf("slept %d times, want 2: once between each pair of calls", len(*slept))
	}
	// The ceiling doubles from base, 100 ms then 200 ms, and max caps it at 150 ms.
	for i, limit := range []time.Duration{100 * time.Millisecond, 150 * time.Millisecond} {
		if d := (*slept)[i]; d < 0 || d > limit {
			t.Errorf("sleep %d = %v, want between 0 and %v", i+1, d, limit)
		}
	}
}

func TestRetryGivesUpAfterAttempts(t *testing.T) {
	slept := fakeSleep(t)
	calls := 0
	err := Retry(context.Background(), 3, time.Millisecond, time.Second, func(context.Context) error {
		calls++
		return errUnavailable
	})
	if !errors.Is(err, errUnavailable) || calls != 3 || len(*slept) != 2 {
		t.Fatalf("Retry = %v after %d calls and %d sleeps, want errUnavailable, 3 and 2", err, calls, len(*slept))
	}
}

func TestPermanentErrorStopsRetries(t *testing.T) {
	slept := fakeSleep(t)
	errNotFound := errors.New("404 Not Found")
	calls := 0
	err := Retry(context.Background(), 5, time.Millisecond, time.Second, func(context.Context) error {
		calls++
		return Permanent(errNotFound)
	})
	if !errors.Is(err, errNotFound) || calls != 1 || len(*slept) != 0 {
		t.Fatalf("Retry = %v after %d calls and %d sleeps, want errNotFound, 1 and 0", err, calls, len(*slept))
	}
}

func TestCancelledContextStopsRetries(t *testing.T) {
	fakeSleep(t)
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	// The caller goes away during the first attempt, as on SIGTERM.
	err := Retry(ctx, 5, time.Millisecond, time.Second, func(context.Context) error {
		calls++
		cancel()
		return errUnavailable
	})
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("Retry = %v after %d calls, want context.Canceled after 1", err, calls)
	}
}

func TestBackoffIsFullJitter(t *testing.T) {
	// Attempt 5 of base 100 ms: the ceiling is 100 ms * 2^5 = 3.2 s.
	const n = 1000
	ceiling := 3200 * time.Millisecond
	var sum time.Duration
	for range n {
		d := backoff(5, 100*time.Millisecond, 10*time.Second)
		if d < 0 || d > ceiling {
			t.Fatalf("backoff = %v, want between 0 and %v", d, ceiling)
		}
		sum += d
	}
	// Full jitter draws evenly from 0 to the ceiling, with a mean of 1.6 s.
	// Without jitter, every value is 3.2 s; with equal jitter, the mean is 2.4 s.
	if mean := sum / n; mean < 1400*time.Millisecond || mean > 1800*time.Millisecond {
		t.Fatalf("mean backoff = %v, want about 1.6 s", mean)
	}
}

// endregion: tests
