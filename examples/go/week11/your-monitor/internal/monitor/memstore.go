package monitor

// region: memstore

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"time"
)

// MemStore is a Store that keeps monitors in a map in memory. It is safe
// for concurrent use: the api serves every request in its own goroutine,
// and mu makes those goroutines take turns. Everything in it is gone when
// the process exits.
type MemStore struct {
	mu       sync.Mutex
	monitors map[int64]Monitor
	lastID   int64
	now      func() time.Time // time.Now, replaced in tests
}

// MemStore must have every method of Store: this line fails to compile
// when one is missing or has the wrong signature.
var _ Store = (*MemStore)(nil)

// NewMemStore returns an empty MemStore. The first monitor gets ID 1.
func NewMemStore() *MemStore {
	return &MemStore{monitors: make(map[int64]Monitor), now: time.Now}
}

// List returns every monitor, oldest first. It returns an empty slice,
// never nil, when the store is empty.
func (s *MemStore) List(ctx context.Context) ([]Monitor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := make([]Monitor, 0, len(s.monitors))
	for _, m := range s.monitors {
		list = append(list, m)
	}
	// Map order is random in Go: sort by ID to return them oldest first.
	slices.SortFunc(list, func(a, b Monitor) int { return cmp.Compare(a.ID, b.ID) })
	return list, nil
}

// Get returns the monitor with the given ID, or ErrNotFound.
func (s *MemStore) Get(ctx context.Context, id int64) (Monitor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.monitors[id]
	if !ok {
		return Monitor{}, ErrNotFound
	}
	return m, nil
}

// Create stores m with the next ID and the current time, and returns the
// stored monitor. It returns ErrDuplicate when a monitor with the same URL
// exists. It ignores the ID and CreatedAt that m carries.
func (s *MemStore) Create(ctx context.Context, m Monitor) (Monitor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.monitors {
		if existing.URL == m.URL {
			return Monitor{}, ErrDuplicate
		}
	}
	s.lastID++
	m.ID = s.lastID
	// UTC, to the microsecond: the precision Postgres stores in Week 8.
	m.CreatedAt = s.now().UTC().Truncate(time.Microsecond)
	s.monitors[m.ID] = m
	return m, nil
}

// Delete removes the monitor with the given ID, or returns ErrNotFound.
func (s *MemStore) Delete(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.monitors[id]; !ok {
		return ErrNotFound
	}
	delete(s.monitors, id)
	return nil
}

// endregion: memstore
