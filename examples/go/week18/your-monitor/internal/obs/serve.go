package obs

import (
	"net/http"
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
