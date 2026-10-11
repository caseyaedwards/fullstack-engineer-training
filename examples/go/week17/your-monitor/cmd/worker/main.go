// Command worker takes check jobs from the queue in Postgres, checks each
// URL with probe.Check, and stores one check_results row per check. It
// runs -workers goroutines that each claim one job at a time, until SIGINT
// or SIGTERM arrives. Week 8 checked every monitor each round; Week 9
// leaves the rounds to cmd/scheduler, which fills the queue. Run it from
// the examples/go directory:
//
//	DATABASE_URL=postgres://monitor:changeme@127.0.0.1:5432/monitor?sslmode=disable \
//	    go run ./week17/your-monitor/cmd/worker

// region: imports
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"math"
	"net/http"
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
	"go.opentelemetry.io/otel/trace"

	"fsp/examples/week17/your-monitor/internal/check"
	"fsp/examples/week17/your-monitor/internal/db"
	"fsp/examples/week17/your-monitor/internal/monitor"
	"fsp/examples/week17/your-monitor/internal/obs"
	"fsp/examples/week17/your-monitor/internal/probe"
	"fsp/examples/week17/your-monitor/internal/queue"
)

// endregion: imports

// region: main

// The defaults of the command-line flags, in one place.
const (
	defaultTimeout = 5 * time.Second
	defaultWorkers = 10
	defaultPoll    = time.Second
)

func main() {
	databaseURL := flag.String("database-url", os.Getenv("DATABASE_URL"), "Postgres connection string; defaults to $DATABASE_URL")
	timeout := flag.Duration("timeout", defaultTimeout, "how long to wait for each check")
	workers := flag.Int("workers", defaultWorkers, "how many checks run at the same time")
	poll := flag.Duration("poll", defaultPoll, "how long a worker waits before it looks again when the queue is empty")
	debugAddr := flag.String("debug-addr", "", "address for /metrics and /healthz, for example 127.0.0.1:6060; empty turns it off")
	pprofAddr := flag.String("pprof-addr", "", "loopback address for the pprof server, for example 127.0.0.1:6061; empty turns it off")
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
	// region: health
	// A job takes up to -timeout, and an idle goroutine looks again after
	// -poll. Three of those in a row with no beat means every goroutine is stuck.
	health := obs.NewHealth(3 * (*timeout + *poll))
	if *debugAddr != "" {
		// Prometheus scrapes /metrics on this address from another
		// container, and the kubelet probes /healthz. pprof stays off it.
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		mux.Handle("GET /healthz", health)
		go func() {
			slog.Error("debug server stopped", "err", obs.NewServer(*debugAddr, mux).ListenAndServe())
		}()
	}
	// endregion: health
	if *pprofAddr != "" {
		srv, err := obs.NewPprofServer(*pprofAddr)
		if err != nil {
			obs.Fatal(err)
		}
		go func() {
			slog.Error("pprof server stopped", "err", srv.ListenAndServe())
		}()
	}
	// endregion: setup

	conns, err := connect(ctx, *databaseURL, *workers)
	if err != nil {
		obs.Fatal(err)
	}
	defer conns.Close()
	if err := db.Migrate(ctx, conns); err != nil {
		obs.Fatal(err)
	}

	checkJob := newCheckFunc(*timeout, newCheckMetrics(prometheus.DefaultRegisterer))
	slog.Info("started", "pid", os.Getpid(), "workers", *workers, "poll", poll.String())
	run(ctx, *workers, *poll, func(ctx context.Context) (bool, error) {
		health.Beat()
		return queue.Work(ctx, conns, checkJob)
	})
	slog.Info("stopped")
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
	// MaxConns is an int32. Outside its range, the conversion below wraps
	// around, and golangci-lint reports it as G115.
	if workers < 1 || workers >= math.MaxInt32 {
		return nil, fmt.Errorf("-workers %d: want 1 to %d", workers, math.MaxInt32-1)
	}
	cfg.MaxConns = int32(workers) + 1
	cfg.ConnConfig.Tracer = obs.QueryTracer{} // a span for each statement of a job
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
// its own deadline, and turns it into a check_results row. It records the
// check in a span, in m and in the log.
func newCheckFunc(timeout time.Duration, m *checkMetrics) queue.CheckFunc {
	return func(ctx context.Context, job queue.Job) monitor.Result {
		// SpanKindClient: the span covers a call this program makes to another.
		ctx, span := otel.Tracer("your-monitor/worker").Start(ctx, "check", trace.WithSpanKind(trace.SpanKindClient))
		defer span.End()
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		checkedAt := time.Now()
		timings, status, err := probe.Check(ctx, job.URL)
		r := check.Result{URL: job.URL, CheckedAt: checkedAt, Status: status,
			Latency: timings.FirstByte, Err: err, Timings: timings}
		result := m.observe(r, time.Since(checkedAt))

		span.SetAttributes(
			attribute.String("url.full", job.URL),
			attribute.String("check.result", result),
			attribute.Int("http.response.status_code", status),
			attribute.Float64("check.dns_ms", ms(timings.DNS)),
			attribute.Float64("check.connect_ms", ms(timings.Connect)),
			attribute.Float64("check.tls_ms", ms(timings.TLS)),
			attribute.Float64("check.first_byte_ms", ms(timings.FirstByte)),
		)
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
		}
		logCheck(ctx, job, r)
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

// logCheck logs one check on one line, with its timings, as Week 4 does,
// and with the trace ID from ctx.
func logCheck(ctx context.Context, job queue.Job, r check.Result) {
	t := r.Timings
	attrs := []any{"job_id", job.ID, "monitor_id", job.MonitorID, "url", r.URL, "up", r.Up(), "status", r.Status,
		"dns_ms", ms(t.DNS), "connect_ms", ms(t.Connect), "tls_ms", ms(t.TLS), "first_byte_ms", ms(t.FirstByte)}
	if r.Err != nil {
		attrs = append(attrs, "err", r.Err.Error())
	}
	slog.InfoContext(ctx, "check", attrs...)
}

// ms turns d into milliseconds, truncated to a tenth.
func ms(d time.Duration) float64 {
	return float64(d.Microseconds()/100) / 10
}

// endregion: logcheck
