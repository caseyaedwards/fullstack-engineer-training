// Command api is the HTTP API of the uptime monitor. Week 6 made it a stub
// with GET /healthz; Week 7 serves the REST API for monitors from
// internal/api, with the monitors kept in memory. Run it from the
// examples/go directory:
//
//	go run ./week07/your-monitor/cmd/api -addr 127.0.0.1:8080
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

	"fsp/examples/week07/your-monitor/internal/api"
	"fsp/examples/week07/your-monitor/internal/monitor"
)

// endregion: imports

// region: handler

// newHandler builds the api the program serves: the routes of internal/api
// over an in-memory store, with one log line per request. The store starts
// empty on every start. Week 8 swaps in Postgres.
func newHandler() http.Handler {
	return api.LogRequests(log.Default(), api.NewHandler(monitor.NewMemStore()))
}

// endregion: handler

// region: main

func main() {
	addr := flag.String("addr", ":8080", "address to listen on; :8080 means port 8080 on every network interface")
	healthcheck := flag.String("healthcheck", "", "URL of a running api's /healthz: exit with 0 if it answers 200, 1 otherwise")
	flag.Parse()

	if *healthcheck != "" {
		if err := checkHealth(*healthcheck); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	log.SetOutput(os.Stdout)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("listening addr=%s", ln.Addr())
	if err := serve(ctx, ln, newHandler()); err != nil {
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
