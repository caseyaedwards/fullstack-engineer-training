package alert

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// region: incidents

// Incident is one row of incidents, with the URL of its monitor.
// ResolvedAt is nil, and null in JSON, while the incident is open.
type Incident struct {
	ID         int64      `json:"id"`
	MonitorID  int64      `json:"monitor_id"`
	URL        string     `json:"url"`
	StartedAt  time.Time  `json:"started_at"`
	ResolvedAt *time.Time `json:"resolved_at"`
	Cause      string     `json:"cause"`
}

// Store reads incidents for the api.
type Store struct {
	DB *pgxpool.Pool
}

// List returns the 100 newest incidents, newest first, or only the open
// ones when openOnly is true. It never returns a nil slice, which encodes
// as [] in JSON, not null.
func (s Store) List(ctx context.Context, openOnly bool) ([]Incident, error) {
	rows, _ := s.DB.Query(ctx, `
		SELECT i.id, i.monitor_id, m.url, i.started_at, i.resolved_at, coalesce(i.cause, '')
		FROM incidents i
		JOIN monitors m ON m.id = i.monitor_id
		WHERE NOT $1 OR i.resolved_at IS NULL
		ORDER BY i.started_at DESC, i.id DESC
		LIMIT 100`, openOnly)
	incidents, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Incident])
	if incidents == nil {
		incidents = []Incident{}
	}
	return incidents, err
}

// endregion: incidents
