// Package week01 holds the small Go examples from the Concepts section of
// Week 1: a struct, a function with two return values, and a loop that
// fills a map from a slice.
package week01

import (
	"fmt"
	"time"
)

// region: result

// Result is what one check of one URL found.
type Result struct {
	URL     string
	Status  int
	Latency time.Duration
}

// endregion: result

// region: classify

// classify turns an HTTP status code into "up" or "down". It returns an
// error for a number that is not an HTTP status code at all.
func classify(status int) (string, error) {
	if status < 100 || status > 599 {
		return "", fmt.Errorf("%d is not an HTTP status code", status)
	}
	if status >= 400 {
		return "down", nil
	}
	return "up", nil
}

// endregion: classify

// region: count

// countStates counts how many results are up and how many are down.
func countStates(results []Result) (map[string]int, error) {
	counts := map[string]int{}
	for _, r := range results {
		state, err := classify(r.Status)
		if err != nil {
			return nil, fmt.Errorf("check of %s: %w", r.URL, err)
		}
		counts[state]++
	}
	return counts, nil
}

// endregion: count
