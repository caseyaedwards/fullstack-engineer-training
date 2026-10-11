package docs

import (
	"fmt"
	"os"
	"regexp"
	"testing"
	"time"
)

// region: example

// ExampleFormatDuration is documentation and a test at once: go doc
// and pkg.go.dev show it under FormatDuration, and go test runs it and
// compares what it prints with the Output comment.
func ExampleFormatDuration() {
	fmt.Println(FormatDuration(1500 * time.Millisecond))
	fmt.Println(FormatDuration(90 * time.Second))
	fmt.Println(FormatDuration(1234567 * time.Microsecond))
	fmt.Println(FormatDuration(2 * time.Minute))
	// Output:
	// 1.5s
	// 1m30s
	// 1.235s
	// 2m
}

// endregion: example

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{1500 * time.Millisecond, "1.5s"},
		{90 * time.Second, "1m30s"},
		{0, "0s"},
		{10 * time.Second, "10s"},
		{10 * time.Minute, "10m"},
		{time.Hour + 10*time.Minute, "1h10m"},
		{456789 * time.Nanosecond, "457µs"},
		{time.Hour, "1h"},
		{time.Hour + 5*time.Second, "1h0m5s"},
		{-1500 * time.Millisecond, "-1.5s"},
	}
	for _, tt := range tests {
		if got := FormatDuration(tt.in); got != tt.want {
			t.Errorf("FormatDuration(%d) = %q, want %q", int64(tt.in), got, tt.want)
		}
	}
}

// TestADRTemplate checks that both regions of adr-template.md, the
// blank template and the filled-in record, hold the four sections of
// an architecture decision record in order.
func TestADRTemplate(t *testing.T) {
	b, err := os.ReadFile("adr-template.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, region := range []string{"template", "record"} {
		re := regexp.MustCompile(`(?s)<!-- region: ` + region + ` -->(.*?)<!-- endregion: ` + region + ` -->`)
		m := re.FindSubmatch(b)
		if m == nil {
			t.Fatalf("region %q not found", region)
		}
		order := regexp.MustCompile(`(?s)^\s*# .+\n.*## Status\n.*## Context\n.*## Decision\n.*## Consequences\n`)
		if !order.Match(m[1]) {
			t.Errorf("region %q: want a # title, then ## Status, Context, Decision and Consequences in that order", region)
		}
	}
}
