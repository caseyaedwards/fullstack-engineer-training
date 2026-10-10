// Command check requests one URL and prints how long each phase took.
//
//	go run ./week04/cmd/check -url https://example.com
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"fsp/examples/week04"
)

// region: main

func main() {
	url := flag.String("url", "", "URL to check")
	timeout := flag.Duration("timeout", 10*time.Second, "how long to wait")
	flag.Parse()
	if *url == "" {
		flag.Usage()
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	timings, status, err := week04.Check(ctx, *url)
	cancel()

	// Print the table even when the check failed: the phases
	// that did happen show how far the request got.
	printTable(os.Stdout, status, timings)
	if err != nil {
		fmt.Fprintln(os.Stderr, "check failed:", err)
		os.Exit(1)
	}
}

// endregion: main

// region: table

// printTable writes the status and one row per phase. A phase
// that did not happen shows as "-".
func printTable(w io.Writer, status int, t week04.Timings) {
	row := func(name, value string) {
		fmt.Fprintf(w, "%-10s %9s\n", name, value)
	}
	ms := func(d time.Duration) string {
		if d == 0 {
			return "-"
		}
		return fmt.Sprintf("%.1f ms", float64(d)/float64(time.Millisecond))
	}

	if status == 0 {
		row("status", "-")
	} else {
		row("status", fmt.Sprintf("%d %s", status, http.StatusText(status)))
	}
	row("DNS", ms(t.DNS))
	row("connect", ms(t.Connect))
	row("TLS", ms(t.TLS))
	row("first byte", ms(t.FirstByte))
	row("total", ms(t.Total))
}

// endregion: table
