// Package databases records one check in two tables, inside one
// transaction: a row in checks and the new last_status of the monitor.
package databases

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// region: schema

// Schema creates the two tables that RecordCheck writes. last_status is
// NULL until the first check of a monitor.
const Schema = `
create table monitors (
    id          bigserial primary key,
    url         text not null,
    last_status text check (last_status in ('up', 'down'))
);

create table checks (
    id         bigserial primary key,
    monitor_id bigint not null references monitors (id),
    checked_at timestamptz not null default now(),
    ok         boolean not null,
    ms         int not null check (ms >= 0)
);
`

// endregion: schema

// region: record

// ErrUnknownMonitor means that no monitor has the id RecordCheck got.
var ErrUnknownMonitor = errors.New("unknown monitor")

// RecordCheck stores one check result: it updates monitors.last_status
// and inserts a row in checks. Both writes commit together, or neither
// does.
func RecordCheck(ctx context.Context, db *pgxpool.Pool, monitorID int64, ok bool, ms int) error {
	status := "down"
	if ok {
		status = "up"
	}
	// BeginFunc runs the function inside BEGIN and COMMIT. When the
	// function returns an error, BeginFunc sends ROLLBACK instead.
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`update monitors set last_status = $2 where id = $1`, monitorID, status)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("monitor %d: %w", monitorID, ErrUnknownMonitor)
		}
		_, err = tx.Exec(ctx,
			`insert into checks (monitor_id, ok, ms) values ($1, $2, $3)`, monitorID, ok, ms)
		return err
	})
}

// endregion: record
