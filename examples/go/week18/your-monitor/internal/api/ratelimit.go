package api

// region: ratelimit

import (
	"context"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/time/rate"

	"fsp/examples/week18/your-monitor/internal/ratelimit"
)

// RateConfig sets the limit on every request from one client. A Limit of
// 0 turns that limit off. Trusted lists the networks of the proxies in
// front of the api, whose X-Forwarded-For the api believes.
type RateConfig struct {
	Limit   rate.Limit // requests per second, per client
	Burst   int        // requests a client can send at once
	Trusted []netip.Prefix
}

// The login limits apply whatever RateConfig says. Each login costs a
// bcrypt check, about 0.3 seconds of CPU. Per client: 5 at once, then one
// every 12 seconds. For all clients together: 3 at once, then one a
// second, which caps bcrypt at under a third of a CPU core per api pod.
const (
	loginBurst    = 5
	loginEvery    = 12 * time.Second
	loginAllBurst = 3
	loginAllRate  = 1 // per second
)

// everyone is the single bucket that loginAll keys every request to.
var everyone = netip.Prefix{}

// RateLimits applies the limits of a RateConfig, plus the login limits.
type RateLimits struct {
	trusted  []netip.Prefix
	client   *ratelimit.Limiter // nil when the limit is off
	login    *ratelimit.Limiter
	loginAll *ratelimit.Limiter
	refused  *prometheus.CounterVec
	now      func() time.Time
}

// NewRateLimits creates the limiters, and registers the counter of
// refused requests with reg.
func NewRateLimits(cfg RateConfig, reg prometheus.Registerer) *RateLimits {
	l := &RateLimits{
		trusted:  cfg.Trusted,
		login:    ratelimit.New(rate.Every(loginEvery), loginBurst),
		loginAll: ratelimit.New(loginAllRate, loginAllBurst),
		refused: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_limited_total",
			Help: "Requests refused with 429, by the limit they hit: client, login or login_all.",
		}, []string{"limit"}),
		now: time.Now,
	}
	if cfg.Limit > 0 {
		l.client = ratelimit.New(cfg.Limit, cfg.Burst)
	}
	reg.MustRegister(l.refused)
	return l
}

// Limit wraps next. It runs before everything else, the span and the log
// line included: a refused request costs a map lookup and a counter, and a
// flood of them writes no log lines.
func (l *RateLimits) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := ratelimit.ClientIP(r, l.trusted)
		key, now := ratelimit.Key(ip), l.now()
		if l.client != nil {
			if wait := l.client.Allow(key, now); wait > 0 {
				l.tooMany(w, "client", wait)
				return
			}
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/login" {
			if wait := l.login.Allow(key, now); wait > 0 {
				l.tooMany(w, "login", wait)
				return
			}
			if wait := l.loginAll.Allow(everyone, now); wait > 0 {
				l.tooMany(w, "login_all", wait)
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), clientKey{}, ip)))
	})
}

// tooMany answers 429 with a Retry-After header in whole seconds,
// rounded up.
func (l *RateLimits) tooMany(w http.ResponseWriter, limit string, wait time.Duration) {
	l.refused.WithLabelValues(limit).Inc()
	secs := max(1, int((wait+time.Second-1)/time.Second))
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	writeError(w, http.StatusTooManyRequests, "too many requests")
}

// clientKey is the context key under which Limit stores the client
// address, for LogRequests.
type clientKey struct{}

// clientFrom returns the client address that Limit found for ctx.
func clientFrom(ctx context.Context) (netip.Addr, bool) {
	ip, ok := ctx.Value(clientKey{}).(netip.Addr)
	return ip, ok && ip.IsValid()
}

// endregion: ratelimit
