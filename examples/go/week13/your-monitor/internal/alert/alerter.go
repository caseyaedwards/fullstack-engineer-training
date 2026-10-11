package alert

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"fsp/examples/week13/your-monitor/internal/monitor"
)

// region: alerter

// Results reads the newest check results of a monitor, newest first.
// db.ResultStore from Week 8 implements it.
type Results interface {
	Recent(ctx context.Context, monitorID int64, limit int) ([]monitor.Result, error)
}

// Alerter opens and resolves incidents from check results, and notifies
// on each change.
type Alerter struct {
	DB        *pgxpool.Pool
	Results   Results
	Notifier  Notifier
	Threshold int // failed results in a row that open an incident
}

// target is one monitor with the ID of its open incident, 0 when none.
type target struct {
	monitorID, openID int64
	url               string
}

// Round applies the rule to every monitor once. It keeps going after a
// failed monitor and returns every error together.
func (a *Alerter) Round(ctx context.Context) error {
	rows, _ := a.DB.Query(ctx, `
		SELECT m.id, m.url, coalesce(i.id, 0)
		FROM monitors m
		LEFT JOIN incidents i ON i.monitor_id = m.id AND i.resolved_at IS NULL
		ORDER BY m.id`)
	targets, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (target, error) {
		var t target
		return t, row.Scan(&t.monitorID, &t.url, &t.openID)
	})
	if err != nil {
		return fmt.Errorf("list monitors: %w", err)
	}

	var errs []error
	for _, t := range targets {
		recent, err := a.Results.Recent(ctx, t.monitorID, a.Threshold)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		switch Decide(recent, t.openID != 0, a.Threshold) {
		case Open:
			// The incident starts at the first of the failed checks.
			err = a.open(ctx, t, recent[a.Threshold-1].CheckedAt, Cause(recent[0]))
		case Resolve:
			err = a.resolve(ctx, t, FirstUp(recent).CheckedAt)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("monitor %d: %w", t.monitorID, err))
		}
	}
	return errors.Join(errs...)
}

// endregion: alerter

// region: open

// open inserts the incident and sends the notification in one
// transaction. When Notify fails, the insert rolls back, and the next
// round tries both again. When another alerter opened the incident first,
// the unique index turns the insert into a no-op, and this alerter sends
// nothing.
func (a *Alerter) open(ctx context.Context, t target, started time.Time, cause string) error {
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	e := Event{Kind: Opened, MonitorID: t.monitorID, URL: t.url, Cause: cause, At: started}
	err = tx.QueryRow(ctx, `
		INSERT INTO incidents (monitor_id, started_at, cause) VALUES ($1, $2, $3)
		ON CONFLICT (monitor_id) WHERE resolved_at IS NULL DO NOTHING
		RETURNING id`, t.monitorID, started, cause).Scan(&e.IncidentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // already open
	}
	if err != nil {
		return err
	}
	if err := a.Notifier.Notify(ctx, e); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// endregion: open

// region: resolve

// resolve closes the open incident and notifies, in one transaction, the
// same way open does.
func (a *Alerter) resolve(ctx context.Context, t target, at time.Time) error {
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	e := Event{Kind: Resolved, IncidentID: t.openID, MonitorID: t.monitorID, URL: t.url, At: at}
	err = tx.QueryRow(ctx, `
		UPDATE incidents SET resolved_at = $2
		WHERE id = $1 AND resolved_at IS NULL
		RETURNING coalesce(cause, '')`, t.openID, at).Scan(&e.Cause)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // another alerter resolved it first
	}
	if err != nil {
		return err
	}
	if err := a.Notifier.Notify(ctx, e); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// endregion: resolve
