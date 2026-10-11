package week01

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestClassify(t *testing.T) {
	for status, want := range map[int]string{200: "up", 301: "up", 399: "up", 400: "down", 404: "down", 500: "down", 599: "down"} {
		got, err := classify(status)
		if err != nil {
			t.Errorf("classify(%d): unexpected error %v", status, err)
		}
		if got != want {
			t.Errorf("classify(%d) = %q, want %q", status, got, want)
		}
	}
	for _, status := range []int{0, 99, 600, -1} {
		got, err := classify(status)
		if err == nil {
			t.Errorf("classify(%d) = %q, want an error", status, got)
		}
		if got != "" {
			t.Errorf("classify(%d) = %q alongside an error, want the zero value \"\"", status, got)
		}
	}
}

func TestCountStates(t *testing.T) {
	results := []Result{
		{URL: "https://example.com", Status: 200, Latency: 112 * time.Millisecond},
		{URL: "https://example.org", Status: 503, Latency: 48 * time.Millisecond},
		{URL: "https://example.net", Status: 301},
	}
	counts, err := countStates(results)
	if err != nil {
		t.Fatalf("countStates: %v", err)
	}
	if counts["up"] != 2 || counts["down"] != 1 || len(counts) != 2 {
		t.Errorf("counts = %v, want map[down:1 up:2]", counts)
	}
}

func TestCountStatesEmpty(t *testing.T) {
	counts, err := countStates(nil)
	if err != nil {
		t.Fatalf("countStates(nil): %v", err)
	}
	// Reading a missing key gives the zero value, not an error.
	if counts["up"] != 0 || len(counts) != 0 {
		t.Errorf("counts = %v, want an empty map", counts)
	}
}

func TestCountStatesWrapsTheError(t *testing.T) {
	_, err := countStates([]Result{{URL: "https://example.com", Status: 200}, {URL: "https://bad.example", Status: 0}})
	if err == nil {
		t.Fatal("countStates: want an error for status 0")
	}
	if !strings.Contains(err.Error(), "check of https://bad.example: 0 is not an HTTP status code") {
		t.Errorf("err = %q, want it to name the URL and the cause", err)
	}
	if errors.Unwrap(err) == nil {
		t.Errorf("err = %q, want it to wrap the error from classify with %%w", err)
	}
}
