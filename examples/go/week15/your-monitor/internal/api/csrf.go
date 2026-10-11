package api

import (
	"net/http"
	"net/url"
)

// region: csrf

// sameOrigin refuses a write that a page on another origin started: a
// cross-site request forgery (CSRF). Reads pass, since they change
// nothing. Browsers name the origin of every request in Sec-Fetch-Site,
// and the Origin header backs it up for browsers from before 2023. A
// request with neither header comes from curl or a program, not from a
// page, and passes: the session check still applies.
func sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		switch site := r.Header.Get("Sec-Fetch-Site"); site {
		case "same-origin", "none": // this origin, or typed by the user
		case "": // an older browser, or not a browser
			if origin := r.Header.Get("Origin"); origin != "" && !hostIs(origin, r.Host) {
				writeError(w, http.StatusForbidden, "cross-origin request refused")
				return
			}
		default: // cross-site, or same-site: a sibling subdomain
			writeError(w, http.StatusForbidden, "cross-origin request refused")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// hostIs reports whether origin, such as https://example.com, names host.
func hostIs(origin, host string) bool {
	u, err := url.Parse(origin)
	return err == nil && u.Host == host
}

// endregion: csrf
