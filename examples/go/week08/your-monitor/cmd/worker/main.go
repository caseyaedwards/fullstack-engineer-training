// Command worker checks every monitor in the database on a pool of worker
// goroutines, stores one check_results row per check, waits for the next
// interval, and does it again, until SIGINT or SIGTERM arrives. Week 8
// replaces the urls.txt file of Weeks 2 to 7 with the monitors table.
// Run it from the examples/go directory:
//
//	DATABASE_URL=postgres://monitor:changeme@127.0.0.1:5432/monitor?sslmode=disable \
//	    go run ./week08/your-monitor/cmd/worker

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

	"fsp/examples/week08/your-monitor/internal/check"
	"fsp/examples/week08/your-monitor/internal/db"
	"fsp/examples/week08/your-monitor/internal/monitor"
	"fsp/examples/week08/your-monitor/internal/pool"
	"fsp/examples/week08/your-monitor/internal/probe"
)

// endregion: imports

// region: main

func main() {
	databaseURL := flag.String("database-url", os.Getenv("DATABASE_URL"), "Postgres connection string; defaults to $DATABASE_URL")
	timeout := flag.Duration("timeout", 5*time.Second, "how long to wait for each check")
	interval := flag.Duration("interval", 30*time.Second, "how long from the start of one round of checks to the next")
	workers := flag.Int("workers", 10, "how many checks run at the same time")
	debugAddr := flag.String("debug-addr", "", "address for the pprof debug server, for example 127.0.0.1:6060; empty turns it off")
	flag.Parse()
	if *databaseURL == "" {
		fmt.Fprintln(os.Stderr, "missing -database-url or DATABASE_URL")
		flag.Usage()
		os.Exit(2)
	}
	if *interval <= 0 || *timeout <= 0 || *workers < 1 {
		fmt.Fprintln(os.Stderr, "-interval and -timeout must be more than 0, and -workers at least 1")
		os.Exit(2)
	}

	log.SetOutput(os.Stdout)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	conns, err := db.Connect(ctx, *databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer conns.Close()
	if err := db.Migrate(ctx, conns); err != nil {
		log.Fatal(err)
	}

	if *debugAddr != "" {
		go func() {
			log.Printf("debug server stopped: %v", http.ListenAndServe(*debugAddr, nil))
		}()
	}

	checkOne := func(ctx context.Context, target string) check.Result {
		ctx, cancel := context.WithTimeout(ctx, *timeout)
		defer cancel()
		checkedAt := time.Now()
		timings, status, err := probe.Check(ctx, target)
		return check.Result{URL: target, CheckedAt: checkedAt, Status: status,
			Latency: timings.FirstByte, Err: err, Timings: timings}
	}

	log.Printf("started pid=%d interval=%s workers=%d", os.Getpid(), *interval, *workers)
	run(ctx, db.NewMonitorStore(conns), db.NewResultStore(conns), *interval, *workers, checkOne)
	log.Print("stopped")
}

// endregion: main

// region: run

// lister and inserter are the parts of the stores that run uses. The
// tests pass fakes; main passes db.MonitorStore and db.ResultStore.
type lister interface {
	List(ctx context.Context) ([]monitor.Monitor, error)
}

type inserter interface {
	Insert(ctx context.Context, r monitor.Result) error
}

// run loads the monitors, checks them through a pool of workers, logs
// and stores the results, logs a summary, waits for the next tick of the
// interval, and does it again. It returns once ctx is cancelled.
func run(ctx context.Context, monitors lister, results inserter, interval time.Duration, workers int, checkOne pool.CheckFunc) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		start := time.Now()
		ms, err := monitors.List(ctx)
		if err != nil {
			log.Printf("round error=%q", err) // try again next round
		}
		ids := make(map[string]int64, len(ms)) // url is unique in monitors
		targets := make([]string, 0, len(ms))
		for _, m := range ms {
			ids[m.URL] = m.ID
			targets = append(targets, m.URL)
		}

		checked := pool.Run(ctx, targets, workers, checkOne)
		for _, r := range checked {
			logCheck(r)
		}
		save(ctx, results, ids, checked)
		log.Printf("round %v duration_ms=%d goroutines=%d",
			summarize(len(targets), checked), time.Since(start).Milliseconds(), runtime.NumGoroutine())

		select {
		case <-ctx.Done():
		case <-ticker.C:
		}
	}
}

// endregion: run

// region: save

// save stores one row per finished check. A cancelled check says nothing
// about the site, so it is not stored. The inserts run even after ctx is
// cancelled, for up to 5 seconds: on SIGTERM, the checks that finished
// before the signal still reach the database.
func save(ctx context.Context, results inserter, ids map[string]int64, checked []check.Result) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	for _, r := range checked {
		if r.Cancelled() {
			continue
		}
		if err := results.Insert(ctx, toResult(ids[r.URL], r)); err != nil {
			log.Printf("save url=%s error=%q", r.URL, err)
		}
	}
}

// toResult turns the outcome of one check into a check_results row.
// A step that did not happen has a zero duration and becomes NULL.
func toResult(monitorID int64, r check.Result) monitor.Result {
	ms := func(d time.Duration) *int {
		if d == 0 {
			return nil
		}
		n := int(d.Milliseconds())
		return &n
	}
	out := monitor.Result{
		MonitorID:   monitorID,
		CheckedAt:   r.CheckedAt,
		Up:          r.Up(),
		DNSMs:       ms(r.Timings.DNS),
		ConnectMs:   ms(r.Timings.Connect),
		TLSMs:       ms(r.Timings.TLS),
		FirstByteMs: ms(r.Timings.FirstByte),
		TotalMs:     ms(r.Timings.Total),
	}
	if r.Status != 0 {
		out.StatusCode = &r.Status
	}
	if r.Err != nil {
		msg := r.Err.Error()
		out.Error = &msg
	}
	return out
}

// endregion: save

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
