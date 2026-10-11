// Command worker is the Week 5 monitor, moved to cmd/worker in Week 6.
// It checks every URL in a file on a pool of worker goroutines, waits for
// the next interval, and checks them all again, until SIGINT or SIGTERM
// arrives. Only its import paths change in the move: check, pool and
// probe now live under internal/. Run it from the examples/go directory:
//
//	go run ./week06/your-monitor/cmd/worker -file week06/your-monitor/urls.txt
//
// Every line from the package clause down matches the Week 5 main.go,
// apart from the import paths; layout_test.go in week06 checks that.

// region: imports
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"fsp/examples/week06/your-monitor/internal/check"
	"fsp/examples/week06/your-monitor/internal/pool"
	"fsp/examples/week06/your-monitor/internal/probe"
)

// endregion: imports

// region: main

func main() {
	file := flag.String("file", "", "file with one URL per line, for example urls.txt")
	timeout := flag.Duration("timeout", 5*time.Second, "how long to wait for each check")
	interval := flag.Duration("interval", 30*time.Second, "how long from the start of one round of checks to the next")
	workers := flag.Int("workers", 10, "how many checks run at the same time")
	debugAddr := flag.String("debug-addr", "", "address for the pprof debug server, for example 127.0.0.1:6060; empty turns it off")
	flag.Parse()
	if *file == "" {
		fmt.Fprintln(os.Stderr, "missing -file")
		flag.Usage()
		os.Exit(2)
	}
	if *interval <= 0 || *timeout <= 0 || *workers < 1 {
		fmt.Fprintln(os.Stderr, "-interval and -timeout must be more than 0, and -workers at least 1")
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

	if *debugAddr != "" {
		go func() {
			log.Printf("debug server stopped: %v", http.ListenAndServe(*debugAddr, nil))
		}()
	}

	checkOne := func(ctx context.Context, target string) check.Result {
		ctx, cancel := context.WithTimeout(ctx, *timeout)
		defer cancel()
		timings, status, err := probe.Check(ctx, target)
		return check.Result{URL: target, Status: status, Latency: timings.FirstByte, Err: err, Timings: timings}
	}

	log.Printf("started pid=%d urls=%d interval=%s workers=%d", os.Getpid(), len(targets), *interval, *workers)
	run(ctx, targets, *interval, *workers, checkOne)
	log.Print("stopped")
}

// endregion: main

// region: run

// run checks every target through a pool of workers, logs the results and
// a summary, waits for the next tick of the interval, and does it again.
// It returns once ctx is cancelled. The checks in flight are cancelled
// with it, and the rest of that round is skipped.
func run(ctx context.Context, targets []string, interval time.Duration, workers int, checkOne pool.CheckFunc) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		start := time.Now()
		results := pool.Run(ctx, targets, workers, checkOne)
		for _, r := range results {
			logCheck(r)
		}
		log.Printf("round %v duration_ms=%d goroutines=%d",
			summarize(len(targets), results), time.Since(start).Milliseconds(), runtime.NumGoroutine())

		select {
		case <-ctx.Done():
		case <-ticker.C:
		}
	}
}

// endregion: run

// region: summary

// summary counts the outcomes of one round.
type summary struct {
	checks, up, down, timeouts, cancelled, skipped int
}

// summarize counts results for a round of total targets. A cancelled
// check is neither up nor down, and a target with no result is skipped.
func summarize(total int, results []check.Result) summary {
	s := summary{checks: len(results), skipped: total - len(results)}
	for _, r := range results {
		switch {
		case r.Cancelled():
			s.cancelled++
		case r.Up():
			s.up++
		default:
			s.down++
			if r.TimedOut() {
				s.timeouts++
			}
		}
	}
	return s
}

func (s summary) String() string {
	return fmt.Sprintf("checks=%d up=%d down=%d timeouts=%d cancelled=%d skipped=%d",
		s.checks, s.up, s.down, s.timeouts, s.cancelled, s.skipped)
}

// endregion: summary

// region: logcheck

// logCheck logs one check with its timings, as Week 4 does.
func logCheck(r check.Result) {
	ms := func(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }
	t := r.Timings
	line := fmt.Sprintf("check url=%s status=%d dns_ms=%.1f connect_ms=%.1f tls_ms=%.1f first_byte_ms=%.1f",
		r.URL, r.Status, ms(t.DNS), ms(t.Connect), ms(t.TLS), ms(t.FirstByte))
	if r.Err != nil {
		line += fmt.Sprintf(" error=%q", r.Err)
	}
	log.Print(line)
}

// endregion: logcheck

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
