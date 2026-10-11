// Command scheduler adds a check job to the queue in Postgres for every
// monitor that is due, every -interval, until SIGINT or SIGTERM arrives.
// Run it from the examples/go directory:
//
//	DATABASE_URL=postgres://monitor:changeme@127.0.0.1:5432/monitor?sslmode=disable \
//	    go run ./week10/your-monitor/cmd/scheduler

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

	"github.com/jackc/pgx/v5/pgxpool"

	"fsp/examples/week10/your-monitor/internal/db"
	"fsp/examples/week10/your-monitor/internal/queue"
)

func main() {
	databaseURL := flag.String("database-url", os.Getenv("DATABASE_URL"), "Postgres connection string; defaults to $DATABASE_URL")
	interval := flag.Duration("interval", 5*time.Second, "how often to look for monitors that are due")
	flag.Parse()
	if *databaseURL == "" {
		fmt.Fprintln(os.Stderr, "missing -database-url or DATABASE_URL")
		os.Exit(2)
	}
	if *interval <= 0 {
		fmt.Fprintln(os.Stderr, "-interval must be more than 0")
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

	log.Printf("started pid=%d interval=%s", os.Getpid(), *interval)
	every(ctx, *interval, func(ctx context.Context) { round(ctx, pool) })
	log.Print("stopped")
}

// endregion: main

// region: round

// round queues the due monitors and logs one line about the queue.
func round(ctx context.Context, pool *pgxpool.Pool) {
	added, err := queue.Enqueue(ctx, pool)
	if err != nil {
		log.Printf("round failed: %v", err)
		return
	}
	queued, oldest, err := queue.Depth(ctx, pool)
	if err != nil {
		log.Printf("round failed: %v", err)
		return
	}
	log.Printf("round enqueued=%d queued=%d oldest_ms=%d", added, queued, oldest.Milliseconds())
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
