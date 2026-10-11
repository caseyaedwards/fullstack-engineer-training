package api

// region: metrics

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics counts and times the requests the api answers.
type Metrics struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

// NewMetrics creates the request metrics and registers them with reg.
// main passes prometheus.DefaultRegisterer; each test passes its own
// prometheus.NewRegistry(), which starts every count at zero.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "HTTP requests answered, by method, route and status code.",
		}, []string{"method", "route", "code"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Time taken to answer HTTP requests, by method and route.",
			Buckets: prometheus.DefBuckets, // 5 ms to 10 s
		}, []string{"method", "route"}),
	}
	reg.MustRegister(m.requests, m.duration)
	return m
}

// Instrument wraps next. After next answers, it counts the request and
// records how long next took.
func (m *Metrics) Instrument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		method, rt := methodLabel(r.Method), route(r)
		m.requests.WithLabelValues(method, rt, strconv.Itoa(rec.status)).Inc()
		m.duration.WithLabelValues(method, rt).Observe(time.Since(start).Seconds())
	})
}

// endregion: metrics

// region: labels

// route returns the pattern of the route that served r, without its
// method: /api/monitors/{id} for GET /api/monitors/42. The mux sets
// r.Pattern when a route matches; a request that matches none gets
// "unmatched". The path itself never becomes a label value: every monitor
// ID, and every path a scanner tries, would make a new series.
func route(r *http.Request) string {
	if r.Pattern == "" {
		return "unmatched"
	}
	if _, path, ok := strings.Cut(r.Pattern, " "); ok {
		return path
	}
	return r.Pattern
}

// methodLabel keeps the method label to the methods HTTP defines. A
// client can send any word as the method, and each new word makes a new
// series.
func methodLabel(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodOptions:
		return method
	}
	return "OTHER"
}

// endregion: labels
