package main

// region: metrics

import "github.com/prometheus/client_golang/prometheus"

// queueMetrics holds the queue as the last round measured it.
type queueMetrics struct {
	queued prometheus.Gauge
	oldest prometheus.Gauge
}

// newQueueMetrics creates the queue gauges and registers them with reg.
func newQueueMetrics(reg prometheus.Registerer) *queueMetrics {
	m := &queueMetrics{
		queued: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "monitor_jobs_queued",
			Help: "Check jobs waiting in the queue, at the last scheduler round.",
		}),
		oldest: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "monitor_jobs_oldest_seconds",
			Help: "How long the oldest waiting job has waited past its run_at, at the last scheduler round.",
		}),
	}
	reg.MustRegister(m.queued, m.oldest)
	return m
}

// endregion: metrics
