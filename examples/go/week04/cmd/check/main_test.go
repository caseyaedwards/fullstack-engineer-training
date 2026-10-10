package main

import (
	"strings"
	"testing"
	"time"

	"fsp/examples/week04"
)

// region: test

func TestPrintTable(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		timings week04.Timings
		want    string
	}{
		{
			name:   "every phase happened",
			status: 200,
			timings: week04.Timings{
				DNS:       12400 * time.Microsecond,
				Connect:   18 * time.Millisecond,
				TLS:       38200 * time.Microsecond,
				FirstByte: 1234500 * time.Microsecond,
				Total:     1240 * time.Millisecond,
			},
			want: lines(
				"status        200 OK",
				"DNS          12.4 ms",
				"connect      18.0 ms",
				"TLS          38.2 ms",
				"first byte 1234.5 ms",
				"total      1240.0 ms",
			),
		},
		{
			name:    "lookup failed, so nothing after DNS happened",
			timings: week04.Timings{DNS: 24 * time.Millisecond},
			want: lines(
				"status             -",
				"DNS          24.0 ms",
				"connect            -",
				"TLS                -",
				"first byte         -",
				"total              -",
			),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			printTable(&b, tt.status, tt.timings)
			if got := b.String(); got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// lines joins rows into the text printTable should write.
func lines(rows ...string) string { return strings.Join(rows, "\n") + "\n" }

// endregion: test
