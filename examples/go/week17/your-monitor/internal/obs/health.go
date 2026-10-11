package obs

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// region: health

// Health answers GET /healthz for a program that runs a loop: the worker,
// the scheduler and the alerter. Their HTTP server keeps answering while
// the loop is stuck, so a /healthz that always says ok proves only that
// the server runs. The loop calls Beat on every pass. Health answers 200
// while the last Beat is at most maxAge old, and 503 once it is older.
type Health struct {
	maxAge time.Duration
	last   atomic.Int64 // Unix time of the last Beat, in nanoseconds
}

// NewHealth returns a Health that counts as fresh for maxAge from now,
// which gives the program maxAge to reach its loop.
func NewHealth(maxAge time.Duration) *Health {
	h := &Health{maxAge: maxAge}
	h.Beat()
	return h
}

// Beat records that the loop is alive. Goroutines can call it at once.
func (h *Health) Beat() { h.last.Store(time.Now().UnixNano()) }

func (h *Health) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	age := time.Since(time.Unix(0, h.last.Load()))
	w.Header().Set("Content-Type", "application/json")
	if age > h.maxAge {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, `{"status":"stuck","last_beat_seconds_ago":%d}`+"\n", int(age.Seconds()))
		return
	}
	fmt.Fprintln(w, `{"status":"ok"}`)
}

// endregion: health

// region: ops-mux

// OpsMux returns the handler for the ops port of a program: GET /metrics
// with every metric in prometheus.DefaultRegisterer, and GET /healthz.
func OpsMux(health http.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.Handler())
	mux.Handle("GET /healthz", health)
	return mux
}

// endregion: ops-mux
