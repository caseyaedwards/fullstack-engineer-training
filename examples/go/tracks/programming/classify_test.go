package programming

import (
	"errors"
	"fmt"
	"testing"
)

// region: test
func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, "ok"},
		{"timeout", ErrTimeout, "timeout"},
		{"wrapped timeout", fmt.Errorf("check example.com: %w", ErrTimeout), "timeout"},
		{"wrapped 503", fmt.Errorf("check example.com: %w", &StatusError{Code: 503}), "http-5xx"},
		{"404", &StatusError{Code: 404}, "http-4xx"},
		{"other", errors.New("connection refused"), "other"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(tt.err); got != tt.want {
				t.Errorf("Classify(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}

// endregion: test

// The walkthrough on the track page: three layers of wrapping, one errors.Is.
func TestClassifyThreeWraps(t *testing.T) {
	err := fmt.Errorf("round 7: %w",
		fmt.Errorf("worker 2: %w",
			fmt.Errorf("check https://example.com: %w", ErrTimeout)))
	if got := Classify(err); got != "timeout" {
		t.Errorf("Classify(%v) = %q, want timeout", err, got)
	}
	want := "round 7: worker 2: check https://example.com: timeout"
	if err.Error() != want {
		t.Errorf("message = %q, want %q", err.Error(), want)
	}
}

// A StatusError outside 400 to 599 is not an HTTP failure class.
func TestClassifyStatusOutsideErrorRange(t *testing.T) {
	if got := Classify(&StatusError{Code: 302}); got != "other" {
		t.Errorf("Classify(302) = %q, want other", got)
	}
}

// Wrapping with %v instead of %w flattens the error into text, and
// errors.Is no longer finds ErrTimeout. The page lists this mistake too.
func TestClassifyPercentVLosesTheChain(t *testing.T) {
	err := fmt.Errorf("check example.com: %v", ErrTimeout)
	if got := Classify(err); got != "other" {
		t.Errorf("Classify(%%v wrap) = %q, want other", got)
	}
}
