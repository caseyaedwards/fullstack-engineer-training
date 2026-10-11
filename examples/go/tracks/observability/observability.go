// Package observability counts and times HTTP requests with Prometheus
// client_golang, the way the uptime monitor api does from Week 13.
package observability

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// region: metrics

// newMetrics creates the request counter and the duration histogram. The
// names match the api of Week 13; the labels here are handler and code.
func newMetrics() (*prometheus.CounterVec, *prometheus.HistogramVec) {
	requests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "HTTP requests answered, by handler and status code.",
	}, []string{"handler", "code"})
	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "Time taken to answer HTTP requests, by handler.",
		Buckets: prometheus.DefBuckets, // 5 ms to 10 s
	}, []string{"handler"})
	return requests, duration
}

// register adds c to reg. When reg already holds a collector with the same
// name and labels, from an earlier call to Instrument, register returns that
// one instead: every handler shares one metric and adds its own series.
func register[C prometheus.Collector](reg prometheus.Registerer, c C) C {
	err := reg.Register(c)
	if err == nil {
		return c
	}
	var already prometheus.AlreadyRegisteredError
	if errors.As(err, &already) {
		return already.ExistingCollector.(C)
	}
	panic(err) // same name, different labels or type: a bug in the program
}

// endregion: metrics

// region: instrument

// Instrument wraps next. After next answers, it counts the request under
// handler and its status code, and records how long next took.
func Instrument(reg prometheus.Registerer, handler string, next http.Handler) http.Handler {
	requests, duration := newMetrics()
	requests = register(reg, requests)
	duration = register(reg, duration)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		requests.WithLabelValues(handler, strconv.Itoa(rec.status)).Inc()
		duration.WithLabelValues(handler).Observe(time.Since(start).Seconds())
	})
}

// endregion: instrument

// region: recorder

// statusRecorder remembers the final status code that next writes. A handler
// that never calls WriteHeader sends 200, which is where status starts.
// Informational codes below 200, such as 103 Early Hints, come before the
// final code and are not recorded.
type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wrote && code >= 200 {
		s.status, s.wrote = code, true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	s.wrote = true // the first Write sends the header, with 200 unless set
	return s.ResponseWriter.Write(b)
}

// Unwrap gives http.ResponseController the original writer, for Flush and
// deadlines.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// endregion: recorder
