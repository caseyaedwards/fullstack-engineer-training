// Package ratelimit limits requests per client with the token buckets of
// golang.org/x/time/rate, and finds the address of the client behind the
// proxies that pass its requests on.
package ratelimit

import (
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// region: client-ip

// ClientIP returns the address of the client that sent r. A proxy that
// passes a request on appends the address it received the request from to
// X-Forwarded-For. ClientIP believes only those proxies: it starts at the
// direct peer, r.RemoteAddr, and walks X-Forwarded-For from the right for
// as long as the address in hand belongs to one of the trusted networks.
// The first address outside them is the client. Anything to its left was
// written by the client itself, and a client can write anything there.
func ClientIP(r *http.Request, trusted []netip.Prefix) netip.Addr {
	peer, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{} // not a TCP connection, as in some tests
	}
	addr := peer.Addr().Unmap().WithZone("")
	if !contains(trusted, addr) {
		return addr // nobody we trust passed this request on
	}
	hops := forwardedFor(r.Header)
	for i := len(hops) - 1; i >= 0; i-- {
		hop, err := netip.ParseAddr(hops[i])
		if err != nil {
			// A trusted proxy writes addresses only. Stop here, and count
			// the request against the last proxy that passed it on.
			return addr
		}
		addr = hop.Unmap().WithZone("")
		if !contains(trusted, addr) {
			return addr
		}
	}
	return addr // every hop is ours: a request from inside the cluster
}

// forwardedFor returns every address in the X-Forwarded-For headers of h,
// left to right. A request can carry the header more than once; the lines
// read as one list, in order.
func forwardedFor(h http.Header) []string {
	var hops []string
	for _, line := range h.Values("X-Forwarded-For") {
		for hop := range strings.SplitSeq(line, ",") {
			hops = append(hops, strings.TrimSpace(hop))
		}
	}
	return hops
}

func contains(nets []netip.Prefix, addr netip.Addr) bool {
	for _, n := range nets {
		if n.Contains(addr) {
			return true
		}
	}
	return false
}

// ParsePrefixes reads a comma-separated list of networks, such as
// "10.42.0.0/16, 192.0.2.7". A bare address stands for itself alone.
func ParsePrefixes(s string) ([]netip.Prefix, error) {
	var nets []netip.Prefix
	for field := range strings.SplitSeq(s, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if !strings.Contains(field, "/") {
			addr, err := netip.ParseAddr(field)
			if err != nil {
				return nil, fmt.Errorf("trusted proxy %q: %w", field, err)
			}
			nets = append(nets, netip.PrefixFrom(addr.Unmap(), addr.Unmap().BitLen()))
			continue
		}
		p, err := netip.ParsePrefix(field)
		if err != nil {
			return nil, fmt.Errorf("trusted proxy %q: %w", field, err)
		}
		nets = append(nets, p.Masked())
	}
	return nets, nil
}

// endregion: client-ip

// region: key

// Key returns the bucket that addr counts against. An IPv4 address is one
// bucket of its own. An IPv6 address counts against its /64 network: an
// internet provider hands each customer at least a /64, which holds 2^64
// addresses, and a client that changes its address inside it stays in
// one bucket.
func Key(addr netip.Addr) netip.Prefix {
	if addr.Is6() {
		p, _ := addr.Prefix(64)
		return p
	}
	return netip.PrefixFrom(addr, addr.BitLen())
}

// endregion: key

// region: limiter

// Limiter keeps one token bucket per key. Each bucket holds up to burst
// tokens and refills at limit tokens per second. A request takes one
// token, and a request that finds the bucket empty is refused.
type Limiter struct {
	mu      sync.Mutex
	limit   rate.Limit
	burst   int
	refill  time.Duration // how long an empty bucket takes to fill up
	max     int           // the most buckets the Limiter keeps
	swept   time.Time
	buckets map[netip.Prefix]*bucket
}

type bucket struct {
	tokens *rate.Limiter
	seen   time.Time // the last request
}

// MaxBuckets caps the memory of one Limiter at a few megabytes.
const MaxBuckets = 100_000

// New returns a Limiter with buckets of burst tokens that refill at limit
// tokens per second. rate.Every(12*time.Second) is one token every 12
// seconds.
func New(limit rate.Limit, burst int) *Limiter {
	return &Limiter{
		limit:   limit,
		burst:   burst,
		refill:  time.Duration(float64(burst) / float64(limit) * float64(time.Second)),
		max:     MaxBuckets,
		buckets: map[netip.Prefix]*bucket{},
	}
}

// Allow takes one token from the bucket of key at now. It returns 0 when
// the request can go ahead. Otherwise it returns how long until the bucket
// holds a token again, the value for a Retry-After header.
func (l *Limiter) Allow(key netip.Prefix, now time.Time) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[key]
	if !ok {
		if len(l.buckets) >= l.max && now.Sub(l.swept) >= l.refill {
			l.sweep(now)
		}
		if len(l.buckets) >= l.max {
			return l.refill // full of active clients: new ones wait
		}
		b = &bucket{tokens: rate.NewLimiter(l.limit, l.burst)}
		l.buckets[key] = b
	}
	b.seen = now
	r := b.tokens.ReserveN(now, 1)
	if delay := r.DelayFrom(now); delay > 0 {
		r.CancelAt(now) // give the token back: a refused request costs nothing
		return delay
	}
	return 0
}

// sweep deletes every bucket that has had time to fill up since its last
// request. A full bucket is the same as a new one, which makes the delete
// invisible to the client. The caller holds mu.
func (l *Limiter) sweep(now time.Time) {
	for key, b := range l.buckets {
		if now.Sub(b.seen) >= l.refill {
			delete(l.buckets, key)
		}
	}
	l.swept = now
}

// endregion: limiter
