// Command api is the HTTP API of the uptime monitor. Week 6 made it a stub
// with GET /healthz; Week 7 serves the REST API for monitors from
// internal/api; Week 8 keeps the monitors and their check results in
// Postgres; Week 9 adds the incidents that cmd/alerter opens. Run it from
// the examples/go directory:
//
//	DATABASE_URL=postgres://monitor:changeme@127.0.0.1:5432/monitor?sslmode=disable \
//	    go run ./week10/your-monitor/cmd/api -addr 127.0.0.1:8080
//
// With -healthcheck, it checks a running api instead of starting one.
// The Compose healthcheck uses that mode, because the container image
// holds no curl or wget.

// region: imports
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"fsp/examples/week10/your-monitor/internal/alert"
	"fsp/examples/week10/your-monitor/internal/api"
	"fsp/examples/week10/your-monitor/internal/db"
)

// endregion: imports

// region: handler

// newHandler builds the api the program serves: the routes of internal/api
// over the Postgres stores, with one log line per request.
func newHandler(pool *pgxpool.Pool) http.Handler {
	return api.LogRequests(log.Default(),
		api.NewHandler(db.NewMonitorStore(pool), db.NewResultStore(pool), alert.Store{DB: pool}))
}

// openDB connects to Postgres and brings the schema up to date.
func openDB(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := db.Connect(ctx, url)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// endregion: handler

// region: main

func main() {
	addr := flag.String("addr", ":8080", "address to listen on; :8080 means port 8080 on every network interface")
	healthcheck := flag.String("healthcheck", "", "URL of a running api's /healthz: exit with 0 if it answers 200, 1 otherwise")
	databaseURL := flag.String("database-url", os.Getenv("DATABASE_URL"), "Postgres connection string; defaults to $DATABASE_URL")
	flag.Parse()

	if *healthcheck != "" {
		if err := checkHealth(*healthcheck); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if *databaseURL == "" {
		fmt.Fprintln(os.Stderr, "missing -database-url or DATABASE_URL")
		os.Exit(2)
	}

	log.SetOutput(os.Stdout)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := openDB(ctx, *databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("listening addr=%s", ln.Addr())
	if err := serve(ctx, ln, newHandler(pool)); err != nil {
		log.Fatal(err)
	}
	log.Print("stopped")
}

// endregion: main

// region: serve

// serve answers requests on ln until ctx is cancelled. Then it stops
// accepting connections, gives the requests in flight up to 5 seconds to
// finish, and returns.
func serve(ctx context.Context, ln net.Listener, h http.Handler) error {
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()

	select {
	case err := <-errc:
		return err // Serve failed on its own, before any shutdown
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// checkHealth gets url and returns an error unless the answer is 200 OK
// within 2 seconds.
func checkHealth(url string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: status %d", url, resp.StatusCode)
	}
	return nil
}

// endregion: serve
