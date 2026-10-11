package obs

import (
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"time"
)

// region: serve

// NewServer returns an HTTP server for the metrics and debug ports. It
// differs from what http.ListenAndServe starts in one setting: a client
// gets 5 seconds to send the request headers. Without that limit, a
// client that sends one header byte a minute keeps a connection and a
// goroutine busy for as long as it likes. golangci-lint reports every
// http.ListenAndServe call as G114 for that reason.
func NewServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
	}
}

// endregion: serve

// region: pprof

// NewPprofServer returns a server for the pprof profiling pages, on a
// mux of its own. The pages show the heap, every goroutine and the
// command line, and /debug/pprof/profile starts a 30-second CPU profile:
// no one outside the machine gets to read them. addr must name a loopback
// host, such as 127.0.0.1:6061, or NewPprofServer returns an error.
//
// Importing net/http/pprof also adds the pages to http.DefaultServeMux.
// No program here serves that mux: each one builds its own.
func NewPprofServer(addr string) (*http.Server, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return nil, fmt.Errorf("pprof address %q: want a loopback host, such as 127.0.0.1", addr)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	return NewServer(addr, mux), nil
}

// endregion: pprof
