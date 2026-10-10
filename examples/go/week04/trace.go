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

// region: types

// Timings holds how long each phase took. A phase that did not
// happen (DNS for an IP address, TLS for plain http) is zero.
// FirstByte and Total count from the start of the request, so
// they include the phases before them.
type Timings struct {
	DNS, Connect, TLS, FirstByte, Total time.Duration
}

// client sends the request. Tests swap in one that trusts
// their test server.
var client = http.DefaultClient

// endregion: types

// region: helpers

// Check makes a GET request to url and reports how long each
// phase took, the HTTP status code, and any error. Redirects are
// not followed. Cancelling ctx stops the request.
func Check(ctx context.Context, url string) (Timings, int, error) {
	var (
		mu                               sync.Mutex // guards the values below
		t                                Timings
		dnsStart, connectStart, tlsStart time.Time
	)
	// The HTTP client calls the hooks from its own goroutine, which
	// can still be running after a timeout has made Check return.
	// Every read and write of t and the start times takes the lock.
	begin := func(from *time.Time) {
		mu.Lock()
		defer mu.Unlock()
		*from = time.Now()
	}
	end := func(d *time.Duration, from *time.Time) {
		mu.Lock()
		defer mu.Unlock()
		*d = time.Since(*from)
	}
	result := func() Timings {
		mu.Lock()
		defer mu.Unlock()
		return t
	}
	// endregion: helpers

	// region: timings
	start := time.Now()
	trace := &httptrace.ClientTrace{
		DNSStart:             func(httptrace.DNSStartInfo) { begin(&dnsStart) },
		DNSDone:              func(httptrace.DNSDoneInfo) { end(&t.DNS, &dnsStart) },
		ConnectStart:         func(string, string) { begin(&connectStart) },
		ConnectDone:          func(string, string, error) { end(&t.Connect, &connectStart) },
		TLSHandshakeStart:    func() { begin(&tlsStart) },
		TLSHandshakeDone:     func(tls.ConnectionState, error) { end(&t.TLS, &tlsStart) },
		GotFirstResponseByte: func() { end(&t.FirstByte, &start) },
	}
	// endregion: timings

	// region: request
	ctx = httptrace.WithClientTrace(ctx, trace)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return result(), 0, err
	}
	// Open a new connection every time. Reusing one would skip DNS,
	// connect and TLS, and a monitor wants all of them on every check.
	req.Close = true

	// Don't follow redirects: report the status this URL returned,
	// and keep the timings to one request on one connection.
	c := *client
	c.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := c.Do(req)
	if err != nil {
		return result(), 0, err
	}
	defer resp.Body.Close()

	// Read the whole body so Total covers the full download.
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return result(), resp.StatusCode, err
	}
	end(&t.Total, &start)
	return result(), resp.StatusCode, nil
}

// endregion: request
