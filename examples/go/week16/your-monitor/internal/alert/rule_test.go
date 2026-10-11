package alert

import (
	"testing"

	"fsp/examples/week16/your-monitor/internal/monitor"
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
	if got := Cause(monitor.Result{StatusCode: 503}); got != "status 503" {
		t.Errorf("Cause = %q, want status 503", got)
	}
	if got := Cause(monitor.Result{Error: "no such host"}); got != "no such host" {
		t.Errorf("Cause = %q, want the error", got)
	}
}
