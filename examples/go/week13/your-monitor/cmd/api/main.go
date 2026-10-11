// Command api is the HTTP API of the uptime monitor. It reads the Postgres
// connection string from DATABASE_URL. Run it from the examples/go
// directory:
//
//	DATABASE_URL=postgres://... go run ./week13/your-monitor/cmd/api -addr 127.0.0.1:8080
//
// With -healthcheck, it checks a running api instead of starting one.
// The Compose healthcheck uses that mode, because the container image
// holds no curl or wget. With -create-user EMAIL, it adds a user with the
// password it reads from stdin, and exits.
//
// With -metrics-addr, it serves its Prometheus metrics on that address.
// With OTEL_EXPORTER_OTLP_ENDPOINT set, it sends a span for each request
// to that OpenTelemetry collector.

// region: imports
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"fsp/examples/week13/your-monitor/internal/alert"
	"fsp/examples/week13/your-monitor/internal/api"
	"fsp/examples/week13/your-monitor/internal/auth"
	"fsp/examples/week13/your-monitor/internal/db"
	"fsp/examples/week13/your-monitor/internal/obs"
)

// endregion: imports

// region: handler

// newHandler builds the api the program serves: the routes of internal/api
// over the stores in Postgres and the auth service. Around them, from the
// outside in: a span, a log line, and the request metrics, for every
// request.
// contract stub: week 08 (the Postgres stores); alert.Store is Week 9.
func newHandler(pool *pgxpool.Pool, users *auth.Service, metrics *api.Metrics) http.Handler {
	h := api.NewHandler(db.NewMonitorStore(pool), db.NewResultStore(pool), alert.Store{DB: pool}, users)
	h = metrics.Instrument(h)
	h = api.LogRequests(slog.Default(), h)
	return api.Trace(h)
}

// endregion: handler

// region: main

func main() {
	addr := flag.String("addr", ":8080", "address to listen on; :8080 means port 8080 on every network interface")
	metricsAddr := flag.String("metrics-addr", "", "address for the Prometheus /metrics endpoint, for example 127.0.0.1:2112; empty turns it off")
	healthcheck := flag.String("healthcheck", "", "URL of a running api's /healthz: exit with 0 if it answers 200, 1 otherwise")
	newUser := flag.String("create-user", "", "add a user with this email, with the password read from stdin, then exit")
	flag.Parse()

	if *healthcheck != "" {
		if err := checkHealth(*healthcheck); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	// region: setup
	slog.SetDefault(obs.NewLogger(os.Stdout, "api"))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdown, err := obs.StartTracing(ctx, "api")
	if err != nil {
		obs.Fatal(err)
	}
	defer shutdown(context.Background())
	if *metricsAddr != "" {
		go serveMetrics(*metricsAddr)
	}

	pool, err := connect(ctx)
	if err != nil {
		obs.Fatal(err)
	}
	// endregion: setup
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		obs.Fatal(err)
	}
	// region: create-user-flag
	users := auth.NewService(db.NewAuthStore(pool))
	if *newUser != "" {
		if isTerminal(os.Stdin) {
			obs.Fatal(fmt.Errorf("-create-user reads the password from a pipe: printf the password into it"))
		}
		if err := createUser(ctx, users, *newUser, os.Stdin, os.Stdout); err != nil {
			obs.Fatal(err)
		}
		return
	}
	// endregion: create-user-flag

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		obs.Fatal(err)
	}
	slog.Info("listening", "addr", ln.Addr().String(), "metrics_addr", *metricsAddr)
	if err := serve(ctx, ln, newHandler(pool, users, api.NewMetrics(prometheus.DefaultRegisterer))); err != nil {
		obs.Fatal(err)
	}
	slog.Info("stopped")
}

// endregion: main

// region: metrics-server

// serveMetrics answers GET /metrics on addr with every metric registered
// with prometheus.DefaultRegisterer: the request metrics, and the Go
// runtime and process metrics that client_golang registers on its own.
// The api serves them on a port of their own, which never faces the
// internet.
func serveMetrics(addr string) {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.Handler())
	slog.Error("metrics server stopped", "err", http.ListenAndServe(addr, mux))
}

// endregion: metrics-server

// region: connect

// connect opens the Postgres pool that DATABASE_URL names, with a span for
// each statement that runs inside a request.
func connect(ctx context.Context) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
	if err != nil {
		return nil, err
	}
	cfg.ConnConfig.Tracer = obs.QueryTracer{}
	return pgxpool.NewWithConfig(ctx, cfg)
}

// endregion: connect

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
