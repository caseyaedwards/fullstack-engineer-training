// Package loadbalancing picks a backend for each request, the way a load
// balancer such as Traefik or nginx does: round robin, or least connections.
package loadbalancing

import (
	"errors"
	"sync"
)

// region: backend

// Backend is one server in the pool. A health checker sets Healthy; the
// proxy raises Active when it opens a request to the backend and lowers it
// when the request ends. In a real balancer, those writes happen while
// other goroutines pick backends: it guards Healthy and Active with its own
// lock or keeps them in atomic values.
type Backend struct {
	Addr    string
	Healthy bool
	Active  int
}

// ErrNoHealthyBackend means every backend failed its health check, or the
// pool is empty. A proxy answers 503 Service Unavailable.
var ErrNoHealthyBackend = errors.New("loadbalancing: no healthy backend")

// endregion: backend

// region: roundrobin

// RoundRobin hands out the healthy backends in turn.
type RoundRobin struct {
	mu       sync.Mutex
	backends []*Backend
	next     int // index of the backend to try first on the next call
}

// NewRoundRobin returns a RoundRobin over bs, starting with bs[0].
func NewRoundRobin(bs []*Backend) *RoundRobin {
	return &RoundRobin{backends: bs}
}

// Next returns the next healthy backend. It tries each backend at most once
// per call, starting after the one it returned last.
func (r *RoundRobin) Next() (*Backend, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for range len(r.backends) {
		b := r.backends[r.next]
		r.next = (r.next + 1) % len(r.backends)
		if b.Healthy {
			return b, nil
		}
	}
	return nil, ErrNoHealthyBackend
}

// endregion: roundrobin

// region: leastconn

// LeastConnections returns the healthy backend with the fewest active
// requests. On a tie, the backend that comes first in bs wins.
func LeastConnections(bs []*Backend) (*Backend, error) {
	var best *Backend
	for _, b := range bs {
		if b.Healthy && (best == nil || b.Active < best.Active) {
			best = b
		}
	}
	if best == nil {
		return nil, ErrNoHealthyBackend
	}
	return best, nil
}

// endregion: leastconn
