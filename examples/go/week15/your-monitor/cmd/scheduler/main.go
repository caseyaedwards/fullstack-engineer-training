// Command scheduler adds a check job to the queue in Postgres for every
// monitor that is due, every -interval, until SIGINT or SIGTERM arrives.
// Run it from the examples/go directory:
//
//	DATABASE_URL=postgres://monitor:changeme@127.0.0.1:5432/monitor?sslmode=disable \
//	    go run ./week15/your-monitor/cmd/scheduler

// region: main
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"fsp/examples/week15/your-monitor/internal/db"
	"fsp/examples/week15/your-monitor/internal/obs"
	"fsp/examples/week15/your-monitor/internal/queue"
)

func main() {
	databaseURL := flag.String("database-url", os.Getenv("DATABASE_URL"), "Postgres connection string; defaults to $DATABASE_URL")
	interval := flag.Duration("interval", 5*time.Second, "how often to look for monitors that are due")
	metricsAddr := flag.String("metrics-addr", "", "address for /metrics and /healthz, for example 127.0.0.1:2113; empty turns it off")
	flag.Parse()
	if *databaseURL == "" {
		fmt.Fprintln(os.Stderr, "missing -database-url or DATABASE_URL")
		os.Exit(2)
	}
	if *interval <= 0 {
		fmt.Fprintln(os.Stderr, "-interval must be more than 0")
		os.Exit(2)
	}

	slog.SetDefault(obs.NewLogger(os.Stdout, "scheduler"))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	// region: health
	// The kubelet probes /healthz on -metrics-addr. A round that fails, or
	// runs out of time, still beats: a restart does not fix a database that
	// is down.
	health := obs.NewHealth(3 * *interval)
	if *metricsAddr != "" {
		go func() {
			slog.Error("metrics server stopped", "err", obs.NewServer(*metricsAddr, obs.OpsMux(health)).ListenAndServe())
		}()
	}
	// endregion: health

	pool, err := db.Connect(ctx, *databaseURL)
	if err != nil {
		obs.Fatal(err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		obs.Fatal(err)
	}

	m := newQueueMetrics(prometheus.DefaultRegisterer)
	slog.Info("started", "pid", os.Getpid(), "interval", interval.String())
	every(ctx, *interval, func(ctx context.Context) {
		// A query that hangs ends with the round, after two intervals, and
		// the round still beats.
		rctx, cancel := context.WithTimeout(ctx, *interval*2)
		defer cancel()
		round(rctx, pool, m)
		health.Beat()
	})
	slog.Info("stopped")
}

// endregion: main

// region: round

// round queues the due monitors, then measures the queue: it logs one line
// and sets the queue gauges in m.
func round(ctx context.Context, pool *pgxpool.Pool, m *queueMetrics) {
	added, err := queue.Enqueue(ctx, pool)
	if err != nil {
		slog.Error("round failed", "err", err)
		return
	}
	queued, oldest, err := queue.Depth(ctx, pool)
	if err != nil {
		slog.Error("round failed", "err", err)
		return
	}
	m.queued.Set(float64(queued))
	m.oldest.Set(oldest.Seconds())
	slog.Info("round", "enqueued", added, "queued", queued, "oldest_ms", oldest.Milliseconds())
}

// every calls f at once, then once per tick of d, until ctx is cancelled.
func every(ctx context.Context, d time.Duration, f func(context.Context)) {
	ticker := time.NewTicker(d)
	defer ticker.Stop()
	for ctx.Err() == nil {
		f(ctx)
		select {
		case <-ctx.Done():
		case <-ticker.C:
		}
	}
}

// endregion: round
