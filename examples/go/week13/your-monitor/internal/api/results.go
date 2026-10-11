package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"fsp/examples/week13/your-monitor/internal/monitor"
)

// region: results

// The limit query parameter: how many results one request returns.
const (
	defaultResultLimit = 50
	maxResultLimit     = 500
)

// listResults serves GET /api/monitors/{id}/results?limit=N: the newest N
// results of one monitor, newest first.
func (h *handler) listResults(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	limit := defaultResultLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maxResultLimit {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("limit must be an integer from 1 to %d, got %q", maxResultLimit, raw))
			return
		}
		limit = n
	}

	// A monitor that does not exist is a 404, not an empty list.
	if _, err := h.store.Get(r.Context(), id); errors.Is(err, monitor.ErrNotFound) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("monitor %d not found", id))
		return
	} else if err != nil {
		internalError(w, r, err)
		return
	}

	results, err := h.results.Recent(r.Context(), id, limit)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if results == nil {
		results = []monitor.Result{} // encode as [], never null
	}
	writeJSON(w, http.StatusOK, results)
}

// endregion: results
