package main

import (
	"testing"
	"time"
)

// TestTimeItRunsInParallel checks that four 500 ms waits on four goroutines
// overlap: one after another, they take 2 s.
func TestTimeItRunsInParallel(t *testing.T) {
	start := time.Now()
	timeIt("wait", 4, func() { time.Sleep(500 * time.Millisecond) })
	if took := time.Since(start); took > 1500*time.Millisecond {
		t.Errorf("four 500 ms waits took %v, want them to overlap", took)
	}
}

func TestCompute(t *testing.T) {
	before := sink.Load()
	compute(10)
	if sink.Load() == before {
		t.Error("compute added nothing to sink")
	}
}
