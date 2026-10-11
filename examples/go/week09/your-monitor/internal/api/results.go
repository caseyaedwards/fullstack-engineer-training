// contract stub: week 08

package api

import (
	"context"
	"net/http"
	"strconv"

	"fsp/examples/week09/your-monitor/internal/monitor"
)

// Results reads check results. db.ResultStore implements it.
type Results interface {
	Recent(ctx context.Context, monitorID int64, limit int) ([]monitor.Result, error)
}

// listResults serves GET /api/monitors/{id}/results?limit=N: default 50,
// at most 500, newest first.
func (h *handler) listResults(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			writeError(w, http.StatusBadRequest, "limit must be an integer from 1 to 500")
			return
		}
		limit = n
	}
	rs, err := h.results.Recent(r.Context(), id, limit)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rs)
}
