package monitor

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// region: validate-test

func TestValidate(t *testing.T) {
	tests := []struct {
		url      string
		interval int
		wantErr  string // "" means valid; otherwise a part of the error text
	}{
		{"https://example.com", 60, ""},
		{"http://example.com:8080/health?full=1", 30, ""},
		{"https://example.com", 3600, ""},
		{"example.com", 60, "must start with http:// or https://"},
		{"ftp://example.com", 60, "must start with http:// or https://"},
		{"https://", 60, "must name a host"},
		{"https://exa mple.com", 60, "not a valid URL"},
		{"", 60, "must start with http:// or https://"},
		{"https://example.com", 29, "between 30 and 3600, got 29"},
		{"https://example.com", 3601, "between 30 and 3600, got 3601"},
		{"https://example.com", 0, "between 30 and 3600, got 0"},
	}
	for _, tt := range tests {
		err := Validate(tt.url, tt.interval)
		switch {
		case tt.wantErr == "" && err != nil:
			t.Errorf("Validate(%q, %d) = %v, want nil", tt.url, tt.interval, err)
		case tt.wantErr != "" && err == nil:
			t.Errorf("Validate(%q, %d) = nil, want an error with %q", tt.url, tt.interval, tt.wantErr)
		case tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr):
			t.Errorf("Validate(%q, %d) = %q, want it to contain %q", tt.url, tt.interval, err, tt.wantErr)
		}
	}
}

// endregion: validate-test

// region: store-test

func TestMemStore(t *testing.T) {
	ctx := context.Background()
	s := NewMemStore()
	at := time.Date(2026, 10, 11, 9, 30, 0, 123456789, time.FixedZone("CEST", 2*60*60))
	s.now = func() time.Time { return at }

	a, err := s.Create(ctx, Monitor{URL: "https://example.com", IntervalSeconds: 60, ID: 99})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != 1 {
		t.Errorf("first ID = %d, want 1: Create ignores the ID it is given", a.ID)
	}
	if want := time.Date(2026, 10, 11, 7, 30, 0, 123456000, time.UTC); !a.CreatedAt.Equal(want) || a.CreatedAt.Location() != time.UTC {
		t.Errorf("CreatedAt = %v, want %v: UTC, cut to the microsecond", a.CreatedAt, want)
	}
	b, _ := s.Create(ctx, Monitor{URL: "https://example.org", IntervalSeconds: 300})

	if _, err := s.Create(ctx, Monitor{URL: "https://example.com", IntervalSeconds: 30}); !errors.Is(err, ErrDuplicate) {
		t.Errorf("Create with a URL that exists: err = %v, want ErrDuplicate", err)
	}
	if got, err := s.Get(ctx, b.ID); err != nil || got != b {
		t.Errorf("Get(%d) = %+v, %v; want %+v, nil", b.ID, got, err, b)
	}
	if list, _ := s.List(ctx); len(list) != 2 || list[0] != a || list[1] != b {
		t.Errorf("List = %+v, want [a b], oldest first", list)
	}

	if err := s.Delete(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete: err = %v, want ErrNotFound", err)
	}
	if err := s.Delete(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second Delete: err = %v, want ErrNotFound", err)
	}
	// IDs are never reused: the next monitor gets 3, not 1.
	if c, _ := s.Create(ctx, Monitor{URL: "https://example.com", IntervalSeconds: 60}); c.ID != 3 {
		t.Errorf("ID after a delete = %d, want 3", c.ID)
	}
}

func TestMemStoreEmptyListIsNotNil(t *testing.T) {
	list, err := NewMemStore().List(context.Background())
	if err != nil || list == nil {
		t.Errorf("List on an empty store = %#v, %v; want an empty, non-nil slice", list, err)
	}
}

// TestMemStoreConcurrent creates 100 monitors from 100 goroutines at
// once. Under go test -race, a missing lock fails this test.
func TestMemStoreConcurrent(t *testing.T) {
	s := NewMemStore()
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			url := "https://example.com/" + strings.Repeat("a", i)
			if _, err := s.Create(context.Background(), Monitor{URL: url, IntervalSeconds: 60}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	list, _ := s.List(context.Background())
	if len(list) != 100 {
		t.Fatalf("got %d monitors, want 100", len(list))
	}
	for i, m := range list {
		if m.ID != int64(i+1) {
			t.Fatalf("IDs are not 1 to 100 without gaps: position %d holds ID %d", i, m.ID)
		}
	}
}

// endregion: store-test
