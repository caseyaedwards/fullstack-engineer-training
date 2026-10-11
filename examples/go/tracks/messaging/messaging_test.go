package messaging

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// region: tests

func TestRedeliveredMessageRunsHandlerOnce(t *testing.T) {
	runs := 0
	in := NewInbox(func(Message) error { runs++; return nil })
	m := Message{ID: "check-42-1001", Body: []byte(`{"monitor_id":42,"ok":false}`)}

	dup, err := in.Process(m)
	if dup || err != nil {
		t.Fatalf("first delivery: duplicate=%v err=%v, want false and nil", dup, err)
	}
	// The broker delivers the same message again, as at-least-once allows.
	dup, err = in.Process(m)
	if !dup || err != nil {
		t.Fatalf("redelivery: duplicate=%v err=%v, want true and nil", dup, err)
	}
	if runs != 1 {
		t.Fatalf("handler ran %d times, want 1", runs)
	}
}

func TestHandlerErrorLeavesMessageUnseen(t *testing.T) {
	fail := errors.New("webhook answered 503")
	runs := 0
	in := NewInbox(func(Message) error {
		runs++
		if runs == 1 {
			return fail
		}
		return nil
	})
	m := Message{ID: "check-42-1002"}

	if dup, err := in.Process(m); dup || !errors.Is(err, fail) {
		t.Fatalf("failing delivery: duplicate=%v err=%v, want false and %v", dup, err, fail)
	}
	if dup, err := in.Process(m); dup || err != nil {
		t.Fatalf("redelivery after the error: duplicate=%v err=%v, want false and nil", dup, err)
	}
	if runs != 2 {
		t.Fatalf("handler ran %d times, want 2: the redelivery must run it again", runs)
	}
	if dup, _ := in.Process(m); !dup {
		t.Fatal("third delivery after a success: duplicate=false, want true")
	}
}

// The first delivery holds the handler open while nine more deliveries of
// the same message arrive. When an Inbox releases its lock during the
// handler, the next delivery passes the seen check and runs the handler a
// second time, before the first delivery records the ID.
func TestConcurrentDeliveriesRunHandlerOnce(t *testing.T) {
	var mu sync.Mutex
	runs := 0
	entered := make(chan struct{})
	release := make(chan struct{})
	in := NewInbox(func(Message) error {
		mu.Lock()
		runs++
		first := runs == 1
		mu.Unlock()
		if first {
			close(entered)
			<-release
		}
		return nil
	})
	m := Message{ID: "check-42-1003"}

	var wg sync.WaitGroup
	dups := make(chan bool, 10)
	deliver := func() {
		defer wg.Done()
		dup, err := in.Process(m)
		if err != nil {
			t.Error(err)
		}
		dups <- dup
	}
	wg.Add(1)
	go deliver()
	<-entered
	for range 9 {
		wg.Add(1)
		go deliver()
	}
	// Give the nine a moment to reach Process. A correct Inbox passes
	// whatever the timing: the nine wait for the lock.
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	close(dups)
	duplicates := 0
	for d := range dups {
		if d {
			duplicates++
		}
	}
	if runs != 1 || duplicates != 9 {
		t.Fatalf("10 deliveries at once: handler ran %d times with %d duplicates, want 1 and 9", runs, duplicates)
	}
}

// endregion: tests
