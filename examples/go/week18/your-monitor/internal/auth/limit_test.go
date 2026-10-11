package auth

import (
	"fmt"
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

// TestLimiterCapsItsMemory fills the map with made-up emails, as an
// attacker can. A real user with no failed attempts still gets in.
func TestLimiterCapsItsMemory(t *testing.T) {
	l := newLimiter(5, time.Minute)
	start := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	l.take("first@example.com", start)
	for i := range maxKeys - 1 {
		l.take(fmt.Sprintf("guess%d@example.com", i), start.Add(time.Second))
	}
	if len(l.entries) != maxKeys {
		t.Fatalf("%d keys, want a full map of %d", len(l.entries), maxKeys)
	}

	if ok, _ := l.take("you@example.com", start.Add(2*time.Second)); !ok {
		t.Fatal("a fresh email is refused while the map is full")
	}
	if len(l.entries) != maxKeys {
		t.Errorf("%d keys after the new one, want %d", len(l.entries), maxKeys)
	}
	if _, found := l.entries["first@example.com"]; found {
		t.Error("the oldest key is still there; want it evicted")
	}

	if ok, _ := l.take("late@example.com", start.Add(time.Minute+time.Second)); !ok {
		t.Fatal("a fresh email is refused after the old keys expired")
	}
	if len(l.entries) != 2 {
		t.Errorf("%d keys after the expired ones went, want 2", len(l.entries))
	}
}
