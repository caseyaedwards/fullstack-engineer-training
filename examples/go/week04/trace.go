// Package week04 times each phase of one HTTP request: DNS, TCP connect,
// TLS handshake and the wait for the first byte of the response.
package week04

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"
)

// Timings holds how long each phase took. A phase that did not happen
// (DNS for an IP address, TLS for plain http, any of them on a reused
// connection) is zero.
type Timings struct {
	DNS, Connect, TLS, FirstByte, Total time.Duration
}

// client sends the request. Tests swap it for a client that trusts their server.
var client = http.DefaultClient

// Check makes a GET request to url and reports how long each phase took, the
// HTTP status code, and any error. Redirects are not followed. Cancelling ctx
// stops the request.
func Check(ctx context.Context, url string) (Timings, int, error) {
	var (
		mu                               sync.Mutex // guards t and the start times, see below
		t                                Timings
		dnsStart, connectStart, tlsStart time.Time
	)
	// The transport dials in its own goroutine, which can still be running after
	// Check returns when ctx is cancelled. These helpers keep that safe.
	begin := func(from *time.Time) { mu.Lock(); *from = time.Now(); mu.Unlock() }
	end := func(d *time.Duration, from *time.Time) { mu.Lock(); *d = time.Since(*from); mu.Unlock() }
	result := func() Timings { mu.Lock(); defer mu.Unlock(); return t }

	start := time.Now()

	// region: timings
	trace := &httptrace.ClientTrace{
		DNSStart:             func(httptrace.DNSStartInfo) { begin(&dnsStart) },
		DNSDone:              func(httptrace.DNSDoneInfo) { end(&t.DNS, &dnsStart) },
		ConnectStart:         func(network, addr string) { begin(&connectStart) },
		ConnectDone:          func(network, addr string, err error) { end(&t.Connect, &connectStart) },
		TLSHandshakeStart:    func() { begin(&tlsStart) },
		TLSHandshakeDone:     func(tls.ConnectionState, error) { end(&t.TLS, &tlsStart) },
		GotFirstResponseByte: func() { end(&t.FirstByte, &start) },
	}
	// endregion: timings

	req, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), http.MethodGet, url, nil)
	if err != nil {
		return result(), 0, err
	}
	// Don't follow redirects: report the status this URL returned, and keep the
	// timings to one request on one connection.
	c := *client
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := c.Do(req)
	if err != nil {
		return result(), 0, err
	}
	defer resp.Body.Close()

	// Read the whole body so Total covers the full download, not just the headers.
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return result(), resp.StatusCode, err
	}
	end(&t.Total, &start)
	return result(), resp.StatusCode, nil
}
