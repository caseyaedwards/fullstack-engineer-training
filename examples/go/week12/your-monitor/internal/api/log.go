package api

// region: log

import (
	"log"
	"net/http"
	"time"
)

// LogRequests wraps next and writes one line per request to logger, after
// next answers: the method, the path, the status code, the size of the
// body in bytes and how long next took.
func LogRequests(logger *log.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		ms := float64(time.Since(start).Microseconds()) / 1000
		logger.Printf("request method=%s path=%s status=%d bytes=%d duration_ms=%.1f",
			r.Method, r.URL.Path, rec.status, rec.bytes, ms)
	})
}

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

// endregion: log
