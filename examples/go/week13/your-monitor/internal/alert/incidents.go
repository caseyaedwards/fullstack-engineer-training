package alert

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// region: incidents

// Incident is one row of incidents. ResolvedAt is nil, and null in JSON,
// while the incident is open.
type Incident struct {
	ID         int64      `json:"id"`
	MonitorID  int64      `json:"monitor_id"`
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
		SELECT id, monitor_id, started_at, resolved_at, coalesce(cause, '')
		FROM incidents
		WHERE NOT $1 OR resolved_at IS NULL
		ORDER BY started_at DESC, id DESC
		LIMIT 100`, openOnly)
	incidents, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Incident])
	if incidents == nil {
		incidents = []Incident{}
	}
	return incidents, err
}

// endregion: incidents
