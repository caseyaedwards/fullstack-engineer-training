package main

import (
	"testing"
	"time"
)

// TestTimeItRunsInParallel checks that four waits on four goroutines take
// about as long as one, not four times as long.
func TestTimeItRunsInParallel(t *testing.T) {
	start := time.Now()
	timeIt("wait", 4, func() { time.Sleep(100 * time.Millisecond) })
	if took := time.Since(start); took > 300*time.Millisecond {
		t.Errorf("four 100 ms waits took %v, want them to overlap", took)
	}
}

func TestCompute(t *testing.T) {
	before := sink.Load()
	compute(10)
	if sink.Load() == before {
		t.Error("compute added nothing to sink")
	}
}
