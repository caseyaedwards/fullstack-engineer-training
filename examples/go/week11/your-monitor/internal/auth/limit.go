package auth

import (
	"sync"
	"time"
)

// region: limiter

// limiter allows max login attempts per key in each window of time. The
// key is the email address, which keeps the limit right behind a proxy:
// behind nginx or the Vite dev server, every request comes from the
// address of the proxy.
type limiter struct {
	mu      sync.Mutex
	max     int
	window  time.Duration
	entries map[string]attempts
}

// attempts counts the tries for one key since start.
type attempts struct {
	start time.Time
	count int
}

// maxKeys caps the memory the limiter uses. Each guess at a new email
// adds a key until its window ends.
const maxKeys = 10_000

func newLimiter(max int, window time.Duration) *limiter {
	return &limiter{max: max, window: window, entries: map[string]attempts{}}
}

// take counts one attempt for key at now. It returns true when the attempt
// is allowed. Otherwise it returns false and how long until the window
// ends. Counting before the password check, under the lock, means that
// 100 parallel guesses get 5 tries, not 100.
func (l *limiter) take(key string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	a, ok := l.entries[key]
	if !ok || now.Sub(a.start) >= l.window {
		if len(l.entries) >= maxKeys {
			l.prune(now)
		}
		if len(l.entries) >= maxKeys {
			return false, l.window // full of fresh keys: refuse until some expire
		}
		a = attempts{start: now}
	}
	if a.count >= l.max {
		return false, a.start.Add(l.window).Sub(now)
	}
	a.count++
	l.entries[key] = a
	return true, 0
}

// reset forgets the attempts for key, after a successful login.
func (l *limiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}

// prune deletes the keys whose window has ended. The caller holds mu.
func (l *limiter) prune(now time.Time) {
	for key, a := range l.entries {
		if now.Sub(a.start) >= l.window {
			delete(l.entries, key)
		}
	}
}

// endregion: limiter
