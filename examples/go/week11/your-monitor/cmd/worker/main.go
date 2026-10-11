// Command worker takes check jobs from the queue in Postgres, checks each
// URL with probe.Check, and stores one check_results row per check. It
// runs -workers goroutines that each claim one job at a time, until SIGINT
// or SIGTERM arrives. Week 8 checked every monitor each round; Week 9
// leaves the rounds to cmd/scheduler, which fills the queue. Run it from
// the examples/go directory:
//
//	DATABASE_URL=postgres://monitor:changeme@127.0.0.1:5432/monitor?sslmode=disable \
//	    go run ./week11/your-monitor/cmd/worker

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
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"fsp/examples/week11/your-monitor/internal/check"
	"fsp/examples/week11/your-monitor/internal/db"
	"fsp/examples/week11/your-monitor/internal/monitor"
	"fsp/examples/week11/your-monitor/internal/probe"
	"fsp/examples/week11/your-monitor/internal/queue"
)

// endregion: imports

// region: main

func main() {
	databaseURL := flag.String("database-url", os.Getenv("DATABASE_URL"), "Postgres connection string; defaults to $DATABASE_URL")
	timeout := flag.Duration("timeout", 5*time.Second, "how long to wait for each check")
	workers := flag.Int("workers", 10, "how many checks run at the same time")
	poll := flag.Duration("poll", time.Second, "how long a worker waits before it looks again when the queue is empty")
	debugAddr := flag.String("debug-addr", "", "address for the pprof debug server, for example 127.0.0.1:6060; empty turns it off")
	flag.Parse()
	if *databaseURL == "" {
		fmt.Fprintln(os.Stderr, "missing -database-url or DATABASE_URL")
		flag.Usage()
		os.Exit(2)
	}
	if *timeout <= 0 || *poll <= 0 || *workers < 1 {
		fmt.Fprintln(os.Stderr, "-timeout and -poll must be more than 0, and -workers at least 1")
		os.Exit(2)
	}

	log.SetOutput(os.Stdout)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	conns, err := connect(ctx, *databaseURL, *workers)
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

	checkJob := newCheckFunc(*timeout)
	log.Printf("started pid=%d workers=%d poll=%s", os.Getpid(), *workers, *poll)
	run(ctx, *workers, *poll, func(ctx context.Context) (bool, error) {
		return queue.Work(ctx, conns, checkJob)
	})
	log.Print("stopped")
}

// endregion: main

// region: connect

// connect opens a connection pool with one connection for each worker
// goroutine, plus one spare, and checks that the database answers, as
// db.Connect does. A worker holds its connection, and the lock on its
// job, for the whole check. With fewer connections, workers wait for a
// free connection instead of checking sites.
func connect(ctx context.Context, url string, workers int) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	cfg.MaxConns = int32(workers) + 1
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	return pool, nil
}

// endregion: connect

// region: check

// newCheckFunc returns the function that checks the URL of one job, with
// its own deadline, logs the check, and turns it into a check_results row.
func newCheckFunc(timeout time.Duration) queue.CheckFunc {
	return func(ctx context.Context, job queue.Job) monitor.Result {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		checkedAt := time.Now()
		timings, status, err := probe.Check(ctx, job.URL)
		r := check.Result{URL: job.URL, CheckedAt: checkedAt, Status: status,
			Latency: timings.FirstByte, Err: err, Timings: timings}
		logCheck(r)
		return toResult(job.MonitorID, r)
	}
}

// endregion: check

// region: run

// run starts n goroutines. Each calls work over and over: at once after a
// job, and after a pause of poll when the queue is empty or work fails.
// run returns once ctx is cancelled and every goroutine has stopped.
func run(ctx context.Context, n int, poll time.Duration, work func(context.Context) (bool, error)) {
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				worked, err := work(ctx)
				if err != nil && ctx.Err() == nil {
					log.Printf("job failed: %v", err)
				}
				if worked && err == nil {
					continue // more jobs can be waiting: look again at once
				}
				select {
				case <-ctx.Done():
				case <-time.After(poll):
				}
			}
		}()
	}
	wg.Wait()
}

// endregion: run

// region: toresult

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

// endregion: toresult

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
