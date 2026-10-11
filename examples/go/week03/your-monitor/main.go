// Command your-monitor checks every URL in a file, waits for the next
// interval, and checks them all again, until SIGINT or SIGTERM arrives.
// It logs one line per check to stdout and exits with 0 after a clean
// shutdown, or with 2 when the command or the file is wrong. Run it from
// the examples/go directory:
//
//	go run ./week03/your-monitor -file week02/your-monitor/urls.txt -interval 30s
//
// The check package is the one from Week 2, unchanged.

// region: imports
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fsp/examples/week02/your-monitor/check"
)

// endregion: imports

// region: main

func main() {
	file := flag.String("file", "", "file with one URL per line, for example urls.txt")
	timeout := flag.Duration("timeout", 5*time.Second, "how long to wait for each response")
	interval := flag.Duration("interval", 30*time.Second, "how long from the start of one round of checks to the next")
	flag.Parse()
	if *file == "" {
		fmt.Fprintln(os.Stderr, "missing -file")
		flag.Usage()
		os.Exit(2)
	}
	if *interval <= 0 {
		fmt.Fprintln(os.Stderr, "-interval must be more than 0")
		os.Exit(2)
	}

	targets, err := readList(*file)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	log.SetOutput(os.Stdout)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	client := &http.Client{Timeout: *timeout}
	checkOne := func(target string) check.Result {
		return check.URL(client, target)
	}

	log.Printf("started pid=%d urls=%d interval=%s", os.Getpid(), len(targets), *interval)
	run(ctx, targets, *interval, checkOne)
	log.Print("stopped")
}

// endregion: main

// region: run

// run checks every target, waits for the next tick of the interval, and
// checks them all again. It returns once ctx is cancelled. A check that
// has started runs to its end, and the rest of that round is skipped.
func run(ctx context.Context, targets []string, interval time.Duration, checkOne func(string) check.Result) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		for i, target := range targets {
			if ctx.Err() != nil {
				log.Printf("shutting down, skipped %d of %d checks in this round", len(targets)-i, len(targets))
				return
			}
			log.Println(checkOne(target))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// endregion: run

// region: read

// readList opens the file at path and parses it with check.ParseList.
func readList(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	targets, err := check.ParseList(f)
	if err != nil {
		return nil, fmt.Errorf("%s:\n%w", path, err)
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("%s: no URLs to check", path)
	}
	return targets, nil
}

// endregion: read
