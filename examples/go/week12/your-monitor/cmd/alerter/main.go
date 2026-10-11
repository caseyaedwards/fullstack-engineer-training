// Command alerter opens an incident when a monitor fails -threshold checks
// in a row, resolves it on the next success, and sends a notification for
// each. It runs a round every -interval until SIGINT or SIGTERM arrives.
// It reads ALERT_WEBHOOK_URL for the webhook; without ALERT_WEBHOOK_URL,
// it writes alerts to its own log. Run it from the examples/go directory:
//
//	DATABASE_URL=postgres://monitor:changeme@127.0.0.1:5432/monitor?sslmode=disable \
//	    go run ./week12/your-monitor/cmd/alerter

// region: main
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fsp/examples/week12/your-monitor/internal/alert"
	"fsp/examples/week12/your-monitor/internal/db"
)

func main() {
	databaseURL := flag.String("database-url", os.Getenv("DATABASE_URL"), "Postgres connection string; defaults to $DATABASE_URL")
	interval := flag.Duration("interval", 10*time.Second, "how often to apply the alert rule to every monitor")
	threshold := flag.Int("threshold", 3, "failed checks in a row that open an incident")
	flag.Parse()
	if *databaseURL == "" {
		fmt.Fprintln(os.Stderr, "missing -database-url or DATABASE_URL")
		os.Exit(2)
	}
	if *interval <= 0 || *threshold < 1 {
		fmt.Fprintln(os.Stderr, "-interval must be more than 0, and -threshold at least 1")
		os.Exit(2)
	}

	log.SetOutput(os.Stdout)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, *databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatal(err)
	}

	a := &alert.Alerter{
		DB:        pool,
		Results:   db.NewResultStore(pool),
		Notifier:  newNotifier(os.Getenv("ALERT_WEBHOOK_URL")),
		Threshold: *threshold,
	}
	log.Printf("started pid=%d interval=%s threshold=%d", os.Getpid(), *interval, *threshold)
	every(ctx, *interval, func(ctx context.Context) {
		if err := a.Round(ctx); err != nil && ctx.Err() == nil {
			log.Printf("round failed: %v", err)
		}
	})
	log.Print("stopped")
}

// endregion: main

// region: notifier

// newNotifier returns a webhook notifier for webhookURL, or a log notifier
// when webhookURL is empty.
func newNotifier(webhookURL string) alert.Notifier {
	if webhookURL == "" {
		log.Print("ALERT_WEBHOOK_URL is empty: alerts go to this log only")
		return alert.Log{Logger: log.Default()}
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
