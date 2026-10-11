package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// region: poll-test

// TestIdleWorkersWaitForPoll runs two workers with the default -poll on a
// queue that stays empty for 200 ms. Each worker looks once, at the
// start, then waits a full poll before it looks again.
func TestIdleWorkersWaitForPoll(t *testing.T) {
	if defaultPoll < 500*time.Millisecond {
		t.Errorf("defaultPoll = %v, want about a second", time.Duration(defaultPoll))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	var looks atomic.Int64
	run(ctx, 2, defaultPoll, func(context.Context) (bool, error) {
		looks.Add(1)
		return false, nil // the queue is empty
	})
	if got := looks.Load(); got != 2 {
		t.Errorf("two idle workers looked at the queue %d times in 200 ms, want 2", got)
	}
}

// endregion: poll-test
