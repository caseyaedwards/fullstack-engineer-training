// Package api is the HTTP API of the uptime monitor: JSON over HTTP for
// creating, reading and deleting monitors. api/openapi.yaml, at the root
// of the repository, describes every route this package serves, and
// openapi_test.go checks that the two agree.
package api

// region: routes

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"fsp/examples/week13/your-monitor/internal/monitor"
)

// handler holds what the route handlers share: the stores.
type handler struct {
	store     monitor.Store
	results   monitor.ResultStore
	incidents Incidents // Week 9
	auth      Auth      // Week 11
}

// The results route is a contract stub: week 08.

// region: auth-routes

// routes maps each route pattern to the method that serves it. Since Go
// 1.22, a pattern starts with a method, and {id} matches one path segment.
// Reads stay public, for the status page. requireUser guards every route
// that changes data.
func (h *handler) routes() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"GET /healthz":              h.healthz,
		"GET /api/monitors":         h.list,
		"POST /api/monitors":        h.requireUser(h.create),
		"GET /api/monitors/{id}":    h.get,
		"DELETE /api/monitors/{id}": h.requireUser(h.delete),

		"GET /api/monitors/{id}/results": h.listResults,
		// region: route
		"GET /api/incidents": h.listIncidents,
		// endregion: route

		"POST /api/login":  h.login,
		"POST /api/logout": h.logout,
		"GET /api/me":      h.requireUser(h.me),
	}
}

// NewHandler returns the API: every route, with JSON error bodies, served
// from the stores. Week 8 adds results, Week 9 incidents and Week 11 the
// auth service.
func NewHandler(store monitor.Store, results monitor.ResultStore, incidents Incidents, auth Auth) http.Handler {
	h := &handler{store: store, results: results, incidents: incidents, auth: auth}
	mux := http.NewServeMux()
	for pattern, fn := range h.routes() {
		mux.HandleFunc(pattern, fn)
	}
	return jsonFallback(mux)
}

// endregion: auth-routes
// endregion: routes

// region: fallback

// jsonFallback serves requests that match no route. ServeMux answers those
// with plain text: 404 when no pattern matches the path, and 405 with an
// Allow header when one matches the path but not the method. jsonFallback
// keeps the status code and the headers, and writes the body as JSON.
func jsonFallback(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallback, pattern := mux.Handler(r)
		if pattern != "" { // a route matches
			mux.ServeHTTP(w, r)
			return
		}
		rec := &statusOnly{header: w.Header(), status: http.StatusOK}
		fallback.ServeHTTP(rec, r)
		writeError(w, rec.status, strings.ToLower(http.StatusText(rec.status)))
	})
}

// statusOnly is a ResponseWriter that shares the headers of the real one,
// records the status code, and drops the body.
type statusOnly struct {
	header http.Header
	status int
}

func (s *statusOnly) Header() http.Header         { return s.header }
func (s *statusOnly) WriteHeader(status int)      { s.status = status }
func (s *statusOnly) Write(b []byte) (int, error) { return len(b), nil }

// endregion: fallback

// region: read

func (h *handler) healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	monitors, err := h.store.List(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	if monitors == nil {
		// A nil slice encodes as null. Clients expect a list: send [].
		monitors = []monitor.Monitor{}
	}
	writeJSON(w, http.StatusOK, monitors)
}

func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	m, err := h.store.Get(r.Context(), id)
	switch {
	case errors.Is(err, monitor.ErrNotFound):
		writeError(w, http.StatusNotFound, fmt.Sprintf("monitor %d not found", id))
	case err != nil:
		internalError(w, r, err)
	default:
		writeJSON(w, http.StatusOK, m)
	}
}

// pathID reads the {id} segment of the path. When it is not an integer,
// pathID answers 400 and returns false.
func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := r.PathValue("id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("id must be an integer, got %q", raw))
		return 0, false
	}
	return id, true
}

// endregion: read

// region: create

// createRequest is the body of POST /api/monitors. The client sends only
// these two fields: the store sets the ID and the creation time.
type createRequest struct {
	URL             string `json:"url"`
	IntervalSeconds int    `json:"interval_seconds"`
}

func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	if !isJSON(r) {
		writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}
	var req createRequest
	if status, err := decodeJSON(w, r, &req); err != nil {
		writeError(w, status, err.Error())
		return
	}
	if err := monitor.Validate(req.URL, req.IntervalSeconds); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	m, err := h.store.Create(r.Context(), monitor.Monitor{URL: req.URL, IntervalSeconds: req.IntervalSeconds})
	switch {
	case errors.Is(err, monitor.ErrDuplicate):
		writeError(w, http.StatusConflict, fmt.Sprintf("a monitor for %s already exists", req.URL))
		return
	case err != nil:
		internalError(w, r, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/api/monitors/%d", m.ID))
	writeJSON(w, http.StatusCreated, m)
}

// endregion: create

// region: delete

func (h *handler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	err := h.store.Delete(r.Context(), id)
	switch {
	case errors.Is(err, monitor.ErrNotFound):
		writeError(w, http.StatusNotFound, fmt.Sprintf("monitor %d not found", id))
	case err != nil:
		internalError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent) // success, and no body
	}
}

// internalError logs err for you and answers 500 with a fixed message.
// The client never sees err: it can hold details such as SQL text.
func internalError(w http.ResponseWriter, r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

// endregion: delete
