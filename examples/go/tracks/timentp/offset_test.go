package timentp

import (
	"strings"
	"testing"
	"time"
)

// at returns a wall-clock time ms milliseconds after a fixed base, with no
// monotonic reading, like a timestamp parsed from an NTP packet.
func at(ms int) time.Time {
	base := time.Date(2026, time.October, 11, 14, 0, 0, 0, time.UTC)
	return base.Add(time.Duration(ms) * time.Millisecond)
}

// region: test
func TestOffset(t *testing.T) {
	// The server clock runs 100 ms ahead. The request takes 10 ms each
	// way, and the server spends 1 ms between receiving and sending.
	offset, delay := Offset(at(0), at(110), at(111), at(21))
	if offset != 100*time.Millisecond {
		t.Errorf("offset = %v, want 100ms", offset)
	}
	if delay != 20*time.Millisecond {
		t.Errorf("delay = %v, want 20ms", delay)
	}
}

func TestNowCarriesAMonotonicReading(t *testing.T) {
	now := time.Now()
	if !strings.Contains(now.String(), "m=") {
		t.Errorf("time.Now() = %q, want an m= monotonic reading", now)
	}
	if strings.Contains(now.Round(0).String(), "m=") {
		t.Errorf("time.Now().Round(0) = %q, want no monotonic reading", now.Round(0))
	}
}

// endregion: test

// With the clocks in step but a slower path out than back, Offset reports
// half the difference as offset: NTP cannot see an asymmetric path.
func TestOffsetAsymmetricPath(t *testing.T) {
	// 30 ms out, 10 ms back, 1 ms on the server, no real offset.
	offset, delay := Offset(at(0), at(30), at(31), at(41))
	if offset != 10*time.Millisecond {
		t.Errorf("offset = %v, want 10ms: half of the 20 ms asymmetry", offset)
	}
	if delay != 40*time.Millisecond {
		t.Errorf("delay = %v, want 40ms", delay)
	}
}

// A server clock behind the client gives a negative offset.
func TestOffsetServerBehind(t *testing.T) {
	offset, _ := Offset(at(0), at(-240), at(-239), at(21))
	if offset != -250*time.Millisecond {
		t.Errorf("offset = %v, want -250ms", offset)
	}
}
