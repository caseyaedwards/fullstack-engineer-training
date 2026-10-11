package main

// region: metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"fsp/examples/week13/your-monitor/internal/check"
)

// checkMetrics counts and times the checks this worker runs.
type checkMetrics struct {
	checks   *prometheus.CounterVec
	duration prometheus.Histogram
}

// newCheckMetrics creates the check metrics and registers them with reg.
func newCheckMetrics(reg prometheus.Registerer) *checkMetrics {
	m := &checkMetrics{
		checks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "monitor_checks_total",
			Help: "Checks run, by result: up, down (the site answered with an error status) or error (no answer).",
		}, []string{"result"}),
		duration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "monitor_check_duration_seconds",
			Help: "Time taken by one check, from the DNS lookup to the end of the body, or to the error.",
			// Bounds at 5 s, the default -timeout, and at 6 s: a check that
			// times out takes just over 5 s and lands in (5, 6], which keeps
			// a p95 of timeouts near 5 s instead of anywhere up to 10 s.
			Buckets: []float64{.01, .025, .05, .1, .25, .5, 1, 2, 3, 4, 5, 6},
		}),
	}
	reg.MustRegister(m.checks, m.duration)
	// Create all three series at 0. A series that appears only with the
	// first failure has no earlier value, and rate() over it shows nothing.
	for _, result := range []string{"up", "down", "error"} {
		m.checks.WithLabelValues(result)
	}
	return m
}

// observe counts r under its result, records how long the check took, and
// returns the result label.
func (m *checkMetrics) observe(r check.Result, took time.Duration) string {
	result := "up"
	switch {
	case r.Err != nil:
		result = "error" // no HTTP answer: DNS, connect, TLS or timeout
	case !r.Up():
		result = "down" // an answer with status 400 or more
	}
	m.checks.WithLabelValues(result).Inc()
	m.duration.Observe(took.Seconds())
	return result
}

// endregion: metrics
