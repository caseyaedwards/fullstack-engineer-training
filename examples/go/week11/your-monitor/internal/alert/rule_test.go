package alert

import (
	"testing"
	"time"

	"fsp/examples/week11/your-monitor/internal/monitor"
)

// region: rule-test

func TestDecide(t *testing.T) {
	up, down := monitor.Result{Up: true}, monitor.Result{Up: false}
	tests := []struct {
		name   string
		recent []monitor.Result // newest first
		open   bool
		want   Action
	}{
		{"no results yet", nil, false, Nothing},
		{"one failure", []monitor.Result{down}, false, Nothing},
		{"two failures", []monitor.Result{down, down}, false, Nothing},
		{"three failures in a row", []monitor.Result{down, down, down}, false, Open},
		{"two failures after a success", []monitor.Result{down, down, up}, false, Nothing},
		{"one success after two failures", []monitor.Result{up, down, down}, false, Nothing},
		{"open, still failing", []monitor.Result{down, down, down}, true, Nothing},
		{"open, first success", []monitor.Result{up, down, down}, true, Resolve},
		{"open, no results left", nil, true, Nothing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Decide(tt.recent, tt.open, 3); got != tt.want {
				t.Errorf("Decide = %v, want %v", got, tt.want)
			}
		})
	}
}

// endregion: rule-test

func TestCause(t *testing.T) {
	code, msg := 503, "no such host"
	if got := Cause(monitor.Result{StatusCode: &code}); got != "status 503" {
		t.Errorf("Cause = %q, want status 503", got)
	}
	if got := Cause(monitor.Result{Error: &msg}); got != "no such host" {
		t.Errorf("Cause = %q, want the error", got)
	}
}

func TestFirstUp(t *testing.T) {
	at := func(m int) time.Time { return time.Date(2026, 10, 11, 9, m, 0, 0, time.UTC) }
	recent := []monitor.Result{ // newest first
		{Up: true, CheckedAt: at(3)}, {Up: true, CheckedAt: at(2)}, {Up: false, CheckedAt: at(1)},
	}
	if got := FirstUp(recent).CheckedAt; !got.Equal(at(2)) {
		t.Errorf("FirstUp = %v, want %v: the first success after the failure", got, at(2))
	}
	if got := FirstUp(recent[:1]).CheckedAt; !got.Equal(at(3)) {
		t.Errorf("FirstUp of one result = %v, want %v", got, at(3))
	}
}
