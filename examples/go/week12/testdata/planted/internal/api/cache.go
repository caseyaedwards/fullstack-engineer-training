package api

import (
	"net/http"
	"strings"
)

// cacheControl lets browsers keep the answers to public reads for five
// minutes. The status page asks for the same lists every few seconds, and
// an answer from the browser cache costs the api and the database nothing.
func cacheControl(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "public, max-age=300")
		}
		next.ServeHTTP(w, r)
	})
}
