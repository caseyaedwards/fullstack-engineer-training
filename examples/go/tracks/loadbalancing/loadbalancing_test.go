package loadbalancing

import (
	"errors"
	"slices"
	"sync"
	"testing"
)

// pool returns three healthy backends on documentation addresses.
func pool() []*Backend {
	return []*Backend{
		{Addr: "192.0.2.10:8080", Healthy: true},
		{Addr: "192.0.2.11:8080", Healthy: true},
		{Addr: "192.0.2.12:8080", Healthy: true},
	}
}

// picks calls Next n times and returns the address of each pick.
func picks(t *testing.T, rr *RoundRobin, n int) []string {
	t.Helper()
	var got []string
	for range n {
		b, err := rr.Next()
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		got = append(got, b.Addr)
	}
	return got
}

// region: test

func TestRoundRobinCycles(t *testing.T) {
	rr := NewRoundRobin(pool())
	want := []string{
		"192.0.2.10:8080", "192.0.2.11:8080", "192.0.2.12:8080",
		"192.0.2.10:8080", "192.0.2.11:8080", "192.0.2.12:8080",
	}
	if got := picks(t, rr, 6); !slices.Equal(got, want) {
		t.Errorf("picks %v, want %v", got, want)
	}
}

func TestRoundRobinSkipsUnhealthy(t *testing.T) {
	bs := pool()
	rr := NewRoundRobin(bs)
	bs[1].Healthy = false // fails its health check

	want := []string{"192.0.2.10:8080", "192.0.2.12:8080", "192.0.2.10:8080", "192.0.2.12:8080"}
	if got := picks(t, rr, 4); !slices.Equal(got, want) {
		t.Errorf("picks %v, want %v", got, want)
	}

	bs[1].Healthy = true // passes again and rejoins the pool
	want = []string{"192.0.2.10:8080", "192.0.2.11:8080", "192.0.2.12:8080"}
	if got := picks(t, rr, 3); !slices.Equal(got, want) {
		t.Errorf("after recovery, picks %v, want %v", got, want)
	}
}

func TestLeastConnectionsPicksLowestActive(t *testing.T) {
	bs := pool()
	bs[0].Active, bs[1].Active, bs[2].Active = 7, 2, 4

	b, err := LeastConnections(bs)
	if err != nil || b != bs[1] {
		t.Fatalf("LeastConnections = %v, %v, want 192.0.2.11:8080 with 2 active", b, err)
	}

	bs[1].Healthy = false // the least busy backend is down: next best is 4
	b, err = LeastConnections(bs)
	if err != nil || b != bs[2] {
		t.Fatalf("LeastConnections = %v, %v, want 192.0.2.12:8080 with 4 active", b, err)
	}
}

func TestNoHealthyBackend(t *testing.T) {
	bs := pool()
	for _, b := range bs {
		b.Healthy = false
	}
	if _, err := NewRoundRobin(bs).Next(); !errors.Is(err, ErrNoHealthyBackend) {
		t.Errorf("RoundRobin.Next error %v, want ErrNoHealthyBackend", err)
	}
	if _, err := LeastConnections(bs); !errors.Is(err, ErrNoHealthyBackend) {
		t.Errorf("LeastConnections error %v, want ErrNoHealthyBackend", err)
	}
	if _, err := NewRoundRobin(nil).Next(); !errors.Is(err, ErrNoHealthyBackend) {
		t.Errorf("empty pool: error %v, want ErrNoHealthyBackend", err)
	}
}

// endregion: test

// TestRoundRobinConcurrent calls Next from many goroutines at once, as the
// request handlers of a proxy do. Under -race, a missing lock fails here, and
// each backend still gets exactly its share.
func TestRoundRobinConcurrent(t *testing.T) {
	rr := NewRoundRobin(pool())
	var (
		mu     sync.Mutex
		counts = map[string]int{}
		wg     sync.WaitGroup
	)
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 30 {
				b, err := rr.Next()
				if err != nil {
					t.Error(err)
					return
				}
				mu.Lock()
				counts[b.Addr]++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	for _, b := range pool() {
		if counts[b.Addr] != 1000 {
			t.Errorf("%s got %d picks, want 1000 of 3000", b.Addr, counts[b.Addr])
		}
	}
}
