package api

import (
	"net/http"
	"strings"
)

// region: cache

// cacheControl tells browsers, and every proxy between them and the api,
// to store no answer of the api. The status page polls to see changes,
// and a stored answer hides them until it expires.
func cacheControl(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

// endregion: cache
