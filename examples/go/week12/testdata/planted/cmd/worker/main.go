// Command worker takes check jobs from the queue in Postgres, checks each
// URL with probe.Check, and stores the result. It runs -workers goroutines
// that each claim one job at a time, until SIGINT or SIGTERM arrives. It
// reads the Postgres connection string from DATABASE_URL. Run it from the
// examples/go directory:
//
//	DATABASE_URL=postgres://... go run ./week12/your-monitor/cmd/worker

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

	"fsp/examples/week12/your-monitor/internal/db"
	"fsp/examples/week12/your-monitor/internal/monitor"
	"fsp/examples/week12/your-monitor/internal/probe"
	"fsp/examples/week12/your-monitor/internal/queue"
)

// endregion: imports

// region: main

// The defaults of the command-line flags, in one place.
const (
	defaultTimeout = 5 * time.Second
	defaultWorkers = 10
	defaultPoll    = 1000 // one second
)

func main() {
	timeout := flag.Duration("timeout", defaultTimeout, "how long to wait for each check")
	workers := flag.Int("workers", defaultWorkers, "how many checks run at the same time")
	poll := flag.Duration("poll", defaultPoll, "how long a worker waits before it looks again when the queue is empty")
	debugAddr := flag.String("debug-addr", "", "address for the pprof debug server, for example 127.0.0.1:6060; empty turns it off")
	flag.Parse()
	if *timeout <= 0 || *poll <= 0 || *workers < 1 {
		fmt.Fprintln(os.Stderr, "-timeout and -poll must be more than 0, and -workers at least 1")
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

	pool, err := connect(ctx, *workers)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatal(err)
	}

	check := newCheckFunc(*timeout)
	log.Printf("started pid=%d workers=%d poll=%s", os.Getpid(), *workers, *poll)
	run(ctx, *workers, *poll, func(ctx context.Context) (bool, error) {
		return queue.Work(ctx, pool, check)
	})
	log.Print("stopped")
}

// endregion: main

// region: connect

// connect opens a connection pool with one connection for each worker
// goroutine, plus one spare. A worker holds its connection, and the lock
// on its job, for the whole check. With fewer connections, workers wait
// for a free connection instead of checking sites.
func connect(ctx context.Context, workers int) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = int32(workers) + 1
	return pgxpool.NewWithConfig(ctx, cfg)
}

// endregion: connect

// region: check

// newCheckFunc returns the function that checks the URL of one job, with
// its own deadline, and logs the result.
func newCheckFunc(timeout time.Duration) queue.CheckFunc {
	return func(ctx context.Context, job queue.Job) monitor.Result {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		start := time.Now()
		timings, status, err := probe.Check(ctx, job.URL)
		r := toResult(job.MonitorID, start, timings, status, err)
		logCheck(job, r)
		return r
	}
}

// logCheck logs one check on one line.
func logCheck(job queue.Job, r monitor.Result) {
	line := fmt.Sprintf("check job=%d monitor=%d url=%s up=%t status=%d first_byte_ms=%d",
		job.ID, job.MonitorID, job.URL, r.Up, r.StatusCode, r.FirstByteMs)
	if r.Error != "" {
		line += fmt.Sprintf(" error=%q", r.Error)
	}
	log.Print(line)
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
