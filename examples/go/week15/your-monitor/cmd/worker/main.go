// Command worker takes check jobs from the queue in Postgres, checks each
// URL with probe.Check, and stores the result. It runs -workers goroutines
// that each claim one job at a time, until SIGINT or SIGTERM arrives. It
// reads the Postgres connection string from DATABASE_URL. Run it from the
// examples/go directory:
//
//	DATABASE_URL=postgres://... go run ./week15/your-monitor/cmd/worker
//
// With -debug-addr, it serves pprof, its Prometheus metrics on /metrics,
// and its health on /healthz, on that address. With
// OTEL_EXPORTER_OTLP_ENDPOINT set, it sends a trace for each job to that
// OpenTelemetry collector.

// region: imports
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	_ "net/http/pprof" //nolint:gosec // G108: pprof answers only on -debug-addr, which you bind to 127.0.0.1
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"fsp/examples/week15/your-monitor/internal/db"
	"fsp/examples/week15/your-monitor/internal/monitor"
	"fsp/examples/week15/your-monitor/internal/obs"
	"fsp/examples/week15/your-monitor/internal/probe"
	"fsp/examples/week15/your-monitor/internal/queue"
)

// endregion: imports

// region: main

func main() {
	timeout := flag.Duration("timeout", 5*time.Second, "how long to wait for each check")
	workers := flag.Int("workers", 10, "how many checks run at the same time")
	poll := flag.Duration("poll", time.Second, "how long a worker waits before it looks again when the queue is empty")
	debugAddr := flag.String("debug-addr", "", "address for pprof, /metrics and /healthz, for example 127.0.0.1:6060; empty turns it off")
	flag.Parse()
	if *timeout <= 0 || *poll <= 0 || *workers < 1 {
		fmt.Fprintln(os.Stderr, "-timeout and -poll must be more than 0, and -workers at least 1")
		os.Exit(2)
	}

	// region: setup
	slog.SetDefault(obs.NewLogger(os.Stdout, "worker"))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdown, err := obs.StartTracing(ctx, "worker")
	if err != nil {
		obs.Fatal(err)
	}
	defer func() {
		// Shutdown sends the spans still in memory. An error here means lost spans.
		if err := shutdown(context.Background()); err != nil {
			slog.Error("tracing shutdown", "err", err)
		}
	}()
	// A job takes up to -timeout, and an idle goroutine looks again after
	// -poll. Three of those in a row with no beat means every goroutine is stuck.
	health := obs.NewHealth(3 * (*timeout + *poll))
	if *debugAddr != "" {
		// pprof registers its pages on http.DefaultServeMux; /metrics and
		// /healthz join them.
		http.Handle("/metrics", promhttp.Handler())
		http.Handle("GET /healthz", health)
		go func() {
			slog.Error("debug server stopped", "err", obs.NewServer(*debugAddr, http.DefaultServeMux).ListenAndServe())
		}()
	}
	// endregion: setup

	pool, err := connect(ctx, *workers)
	if err != nil {
		obs.Fatal(err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		obs.Fatal(err)
	}

	check := newCheckFunc(*timeout, newCheckMetrics(prometheus.DefaultRegisterer))
	slog.Info("started", "pid", os.Getpid(), "workers", *workers, "poll", poll.String())
	run(ctx, *workers, *poll, func(ctx context.Context) (bool, error) {
		health.Beat()
		return queue.Work(ctx, pool, check)
	})
	slog.Info("stopped")
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
	// MaxConns is an int32. Outside its range, the conversion below wraps
	// around, and golangci-lint reports it as G115.
	if workers < 1 || workers >= math.MaxInt32 {
		return nil, fmt.Errorf("-workers %d: want 1 to %d", workers, math.MaxInt32-1)
	}
	cfg.MaxConns = int32(workers) + 1
	cfg.ConnConfig.Tracer = obs.QueryTracer{} // a span for each statement of a job
	return pgxpool.NewWithConfig(ctx, cfg)
}

// endregion: connect

// region: check

// newCheckFunc returns the function that checks the URL of one job, with
// its own deadline. It records the check in a span, in m and in the log.
func newCheckFunc(timeout time.Duration, m *checkMetrics) queue.CheckFunc {
	return func(ctx context.Context, job queue.Job) monitor.Result {
		ctx, span := otel.Tracer("your-monitor/worker").Start(ctx, "check")
		defer span.End()
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		start := time.Now()
		timings, status, err := probe.Check(ctx, job.URL)
		r := toResult(job.MonitorID, start, timings, status, err)
		result := m.observe(r, time.Since(start))

		span.SetAttributes(
			attribute.String("url.full", job.URL),
			attribute.String("check.result", result),
			attribute.Int("http.response.status_code", r.StatusCode),
			attribute.Int("check.dns_ms", r.DNSMs),
			attribute.Int("check.connect_ms", r.ConnectMs),
			attribute.Int("check.tls_ms", r.TLSMs),
			attribute.Int("check.first_byte_ms", r.FirstByteMs),
		)
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
		}
		logCheck(ctx, job, r)
		return r
	}
}

// logCheck logs one check on one line, with the trace ID from ctx.
func logCheck(ctx context.Context, job queue.Job, r monitor.Result) {
	attrs := []any{"job", job.ID, "monitor", job.MonitorID, "url", job.URL,
		"up", r.Up, "status", r.StatusCode, "first_byte_ms", r.FirstByteMs}
	if r.Error != "" {
		attrs = append(attrs, "error", r.Error)
	}
	slog.InfoContext(ctx, "check", attrs...)
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
					slog.Error("job failed", "err", err)
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
