package api

// region: log

import (
	"log/slog"
	"net/http"
	"time"
)

// LogRequests wraps next and logs one line per request to logger, after
// next answers: the method, the route, the path, the status code, the size
// of the body in bytes, how long next took and, behind RateLimits.Limit,
// the client address. It logs with the request
// context, which adds the trace ID when Trace runs before it.
func LogRequests(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		attrs := []any{"method", r.Method, "route", route(r), "path", r.URL.Path, "status", rec.status,
			"bytes", rec.bytes, "duration_ms", float64(time.Since(start).Microseconds()) / 1000}
		if ip, ok := clientFrom(r.Context()); ok {
			attrs = append(attrs, "client", ip.String()) // Week 17: set by RateLimits.Limit
		}
		logger.InfoContext(r.Context(), "request", attrs...)
	})
}

// endregion: log

// statusRecorder passes everything on to the ResponseWriter it embeds, and
// notes the status code and the number of body bytes on the way. A handler
// that never calls WriteHeader answers 200, which is why status starts there.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusRecorder) WriteHeader(status int) {
	s.status = status
	s.ResponseWriter.WriteHeader(status)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}
