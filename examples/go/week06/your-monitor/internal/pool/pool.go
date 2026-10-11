// region: types

// Package pool checks a list of URLs on a fixed number of goroutines.
package pool

import (
	"context"
	"sync"

	"fsp/examples/week06/your-monitor/internal/check"
)

// CheckFunc checks one URL. It must return soon after ctx is cancelled.
type CheckFunc func(ctx context.Context, target string) check.Result

// endregion: types

// region: run

// Run checks every target with checkOne on a pool of workers goroutines,
// and returns one result per check, in the order the checks finish.
// Once ctx is cancelled, Run starts no new checks and returns as soon as
// the checks in flight return. A target that was never checked has no
// result. Run panics when workers is less than 1.
func Run(ctx context.Context, targets []string, workers int, checkOne CheckFunc) []check.Result {
	if workers < 1 {
		panic("pool.Run: workers must be at least 1")
	}
	jobs := make(chan string)
	results := make(chan check.Result)
	// endregion: run

	// region: workers
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for target := range jobs {
				if ctx.Err() != nil {
					continue // cancelled: drain the jobs without checking them
				}
				results <- checkOne(ctx, target)
			}
		}()
	}
	// endregion: workers

	// region: feed
	go func() {
		defer close(jobs)
		for _, target := range targets {
			select {
			case jobs <- target:
			case <-ctx.Done():
				return
			}
		}
	}()
	// endregion: feed

	// region: collect
	go func() {
		wg.Wait()
		close(results)
	}()

	var out []check.Result
	for r := range results {
		out = append(out, r)
	}
	return out
}

// endregion: collect
