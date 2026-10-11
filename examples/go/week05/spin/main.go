// region: spin

// Command spin times goroutines that wait, as a network check does, and
// goroutines that compute, which need a CPU each.
// Compare the times with GOMAXPROCS=1 and without it:
//
//	go run .
//	GOMAXPROCS=1 go run .
package main

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	fmt.Printf("GOMAXPROCS=%d CPUs=%d\n", runtime.GOMAXPROCS(0), runtime.NumCPU())
	wait := func() { time.Sleep(500 * time.Millisecond) }
	work := func() { compute(200_000_000) }
	timeIt("wait   ", 1, wait)
	timeIt("wait   ", 4, wait)
	timeIt("compute", 1, work)
	timeIt("compute", 4, work)
}

// timeIt runs f on n goroutines at once and prints how long they take.
func timeIt(name string, n int, f func()) {
	start := time.Now()
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f()
		}()
	}
	wg.Wait()
	fmt.Printf("%s goroutines=%d took %v\n", name, n, time.Since(start).Round(10*time.Millisecond))
}

// sink keeps the compiler from removing the loop in compute. It is
// atomic because several goroutines add to it at once.
var sink atomic.Uint64

// compute keeps one CPU busy for n steps of a random number generator.
func compute(n int) {
	x := uint64(1)
	for range n {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
	}
	sink.Add(x)
}

// endregion: spin
