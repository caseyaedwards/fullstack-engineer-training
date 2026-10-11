package auth

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// region: limiter-test

func TestLimiter(t *testing.T) {
	l := newLimiter(5, 15*time.Minute)
	start := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)

	for i := range 5 {
		if ok, _ := l.take("you@example.com", start.Add(time.Duration(i)*time.Second)); !ok {
			t.Fatalf("attempt %d refused, want allowed", i+1)
		}
	}
	ok, wait := l.take("you@example.com", start.Add(time.Minute))
	if ok || wait != 14*time.Minute {
		t.Errorf("attempt 6 after 1 minute: ok %v, wait %v; want refused, 14m0s", ok, wait)
	}
	if ok, _ := l.take("other@example.com", start.Add(time.Minute)); !ok {
		t.Error("another email is refused: the limit must count per email")
	}
	if ok, _ := l.take("you@example.com", start.Add(15*time.Minute)); !ok {
		t.Error("refused after the 15-minute window ended")
	}
	l.reset("you@example.com")
	if _, found := l.entries["you@example.com"]; found {
		t.Error("reset left the attempts in place")
	}
}

// TestLimiterUnderParallelGuesses fires 100 attempts at once. The race
// detector checks the lock, and exactly 5 get through.
func TestLimiterUnderParallelGuesses(t *testing.T) {
	l := newLimiter(5, 15*time.Minute)
	now := time.Now()
	var allowed atomic.Int32
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, _ := l.take("you@example.com", now); ok {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	if n := allowed.Load(); n != 5 {
		t.Errorf("%d of 100 parallel attempts allowed, want 5", n)
	}
}

// endregion: limiter-test

func TestLimiterCapsItsMemory(t *testing.T) {
	l := newLimiter(5, time.Minute)
	now := time.Now()
	for i := range maxKeys {
		l.take(string(rune(i)), now)
	}
	if ok, _ := l.take("new@example.com", now); ok {
		t.Error("a new key is allowed while the map is full of fresh keys")
	}
	if ok, _ := l.take("new@example.com", now.Add(time.Minute)); !ok {
		t.Error("a new key is refused after the old keys expired")
	}
	if len(l.entries) != 1 {
		t.Errorf("%d keys after the prune, want 1", len(l.entries))
	}
}
