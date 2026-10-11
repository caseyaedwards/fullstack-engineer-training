package api

import "net/http"

// secureHeaders adds the security headers that the OWASP REST Security
// Cheat Sheet lists for an API to every answer:
//   - X-Content-Type-Options: nosniff stops a browser from guessing a
//     type other than the Content-Type the api sends.
//   - X-Frame-Options: DENY and the frame-ancestors part of the
//     Content-Security-Policy stop another site from showing an answer
//     inside a frame of its own page.
func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", "frame-ancestors 'none'")
	})
}
