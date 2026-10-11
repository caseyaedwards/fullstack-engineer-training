// Command alerter opens an incident when a monitor fails -threshold checks
// in a row, resolves it on the next success, and sends a notification for
// each. It runs a round every -interval until SIGINT or SIGTERM arrives.
// It reads DATABASE_URL, and ALERT_WEBHOOK_URL for the webhook; without
// ALERT_WEBHOOK_URL, it writes alerts to its own log. With -metrics-addr,
// it serves /metrics and /healthz on that address. Run it from the
// examples/go directory:
//
//	DATABASE_URL=postgres://... go run ./week15/your-monitor/cmd/alerter

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

	"fsp/examples/week15/your-monitor/internal/alert"
	"fsp/examples/week15/your-monitor/internal/db"
	"fsp/examples/week15/your-monitor/internal/obs"
)

func main() {
	interval := flag.Duration("interval", 10*time.Second, "how often to apply the alert rule to every monitor")
	threshold := flag.Int("threshold", 3, "failed checks in a row that open an incident")
	metricsAddr := flag.String("metrics-addr", "", "address for /metrics and /healthz, for example 127.0.0.1:2114; empty turns it off")
	flag.Parse()
	if *interval <= 0 || *threshold < 1 {
		fmt.Fprintln(os.Stderr, "-interval must be more than 0, and -threshold at least 1")
		os.Exit(2)
	}

	slog.SetDefault(obs.NewLogger(os.Stdout, "alerter"))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	// region: health
	health := obs.NewHealth(3 * *interval)
	if *metricsAddr != "" {
		go func() {
			slog.Error("metrics server stopped", "err", obs.NewServer(*metricsAddr, obs.OpsMux(health)).ListenAndServe())
		}()
	}
	// endregion: health

	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		obs.Fatal(err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		obs.Fatal(err)
	}

	a := &alert.Alerter{
		DB:        pool,
		Results:   db.NewResultStore(pool),
		Notifier:  newNotifier(os.Getenv("ALERT_WEBHOOK_URL")),
		Threshold: *threshold,
	}
	slog.Info("started", "pid", os.Getpid(), "interval", interval.String(), "threshold", *threshold)
	every(ctx, *interval, func(ctx context.Context) {
		// A query that hangs ends with the round, after two intervals, and
		// the round still beats.
		rctx, cancel := context.WithTimeout(ctx, *interval*2)
		defer cancel()
		if err := a.Round(rctx); err != nil && ctx.Err() == nil {
			slog.Error("round failed", "err", err)
		}
		health.Beat()
	})
	slog.Info("stopped")
}

// endregion: main

// region: notifier

// newNotifier returns a webhook notifier for webhookURL, or a log notifier
// when webhookURL is empty.
func newNotifier(webhookURL string) alert.Notifier {
	if webhookURL == "" {
		slog.Warn("ALERT_WEBHOOK_URL is empty: alerts go to this log only")
		return alert.Log{Logger: slog.Default()}
	}
	return alert.Webhook{URL: webhookURL}
}

// endregion: notifier

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
