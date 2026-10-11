package api

import (
	"context"
	"net/http"
	"strconv"

	"fsp/examples/week15/your-monitor/internal/alert"
)

// region: incidents

// Incidents lists incidents. alert.Store implements it.
type Incidents interface {
	List(ctx context.Context, openOnly bool) ([]alert.Incident, error)
}

// listIncidents serves GET /api/incidents. With ?open=true, it lists only
// the incidents that are still open.
func (h *handler) listIncidents(w http.ResponseWriter, r *http.Request) {
	openOnly := false
	if v := r.URL.Query().Get("open"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "open must be true or false")
			return
		}
		openOnly = b
	}
	incidents, err := h.incidents.List(r.Context(), openOnly)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, incidents)
}

// endregion: incidents
