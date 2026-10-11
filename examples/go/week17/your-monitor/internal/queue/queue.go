// Package queue keeps check jobs in the check_jobs table of Postgres. The
// scheduler adds a job for each monitor that is due, and workers claim
// jobs with SELECT ... FOR UPDATE SKIP LOCKED, which hands each job to
// exactly one worker at a time.
package queue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"fsp/examples/week17/your-monitor/internal/monitor"
)

// region: enqueue

// enqueueSQL marks every due monitor as enqueued and adds one job for
// each, in one statement. A monitor is due when the scheduler has never
// queued it, or last queued it interval_seconds ago or longer, and it has
// no job still waiting. Every time comes from the database clock, now().
const enqueueSQL = `
WITH due AS (
    UPDATE monitors m
    SET last_enqueued_at = now()
    WHERE (m.last_enqueued_at IS NULL
           OR m.last_enqueued_at <= now() - make_interval(secs => m.interval_seconds))
      AND NOT EXISTS (SELECT 1 FROM check_jobs j WHERE j.monitor_id = m.id)
    RETURNING m.id
)
INSERT INTO check_jobs (monitor_id, run_at)
SELECT id, now() FROM due`

// Enqueue adds a job for every due monitor and returns how many it added.
func Enqueue(ctx context.Context, db *pgxpool.Pool) (int64, error) {
	tag, err := db.Exec(ctx, enqueueSQL)
	if err != nil {
		return 0, fmt.Errorf("enqueue: %w", err)
	}
	return tag.RowsAffected(), nil
}

// endregion: enqueue

// region: depth

// Depth returns how many jobs wait in the queue, and how long the oldest
// of them has waited past its run_at. A growing age means the workers
// fall behind.
func Depth(ctx context.Context, db *pgxpool.Pool) (queued int, oldest time.Duration, err error) {
	var seconds float64
	err = db.QueryRow(ctx, `
		SELECT count(*), coalesce(extract(epoch FROM now() - min(run_at)), 0)::float8
		FROM check_jobs`).Scan(&queued, &seconds)
	return queued, time.Duration(seconds * float64(time.Second)), err
}

// endregion: depth

// region: claim

// Job is one check waiting to run.
type Job struct {
	ID        int64
	MonitorID int64
	URL       string
	Attempts  int
}

// CheckFunc runs the check for one job and returns its result. It must
// return soon after ctx is cancelled.
type CheckFunc func(ctx context.Context, job Job) monitor.Result

// claimSQL picks the oldest due job and locks its row. SKIP LOCKED passes
// over rows that another transaction has locked, which means another
// worker holds that job: this worker takes the next one instead of
// waiting. FOR UPDATE OF j locks the job row only, not the monitor.
const claimSQL = `
SELECT j.id, j.monitor_id, m.url, j.attempts
FROM check_jobs j
JOIN monitors m ON m.id = j.monitor_id
WHERE j.run_at <= now()
ORDER BY j.run_at, j.id
LIMIT 1
FOR UPDATE OF j SKIP LOCKED`

// Work claims one due job, runs check on it, and stores the result and
// deletes the job in the same transaction. It returns false when no job is
// due. If anything fails before the commit, the transaction rolls back and
// the job goes back to the queue.
func Work(ctx context.Context, db *pgxpool.Pool, check CheckFunc) (bool, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return false, err
	}
	// Rollback does nothing after a successful Commit.
	defer tx.Rollback(ctx)

	var job Job
	err = tx.QueryRow(ctx, claimSQL).Scan(&job.ID, &job.MonitorID, &job.URL, &job.Attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim: %w", err)
	}

	// region: job-span
	// The trace of a job starts once the job is claimed. A poll that finds
	// no job starts no trace. Every span below, SQL included, joins this one.
	ctx, span := otel.Tracer("your-monitor/queue").Start(ctx, "job", trace.WithAttributes(
		attribute.Int64("job.id", job.ID),
		attribute.Int64("monitor.id", job.MonitorID),
		attribute.Int("job.attempts", job.Attempts),
	))
	defer span.End()
	// endregion: job-span

	if err := runAndStore(ctx, tx, job, check); err != nil {
		span.SetStatus(codes.Error, err.Error())
		tx.Rollback(ctx)
		if ctx.Err() != nil {
			return true, err // shutting down: the job stays as it is for the next worker
		}
		return true, errors.Join(err, retry(ctx, db, job))
	}
	return true, tx.Commit(ctx)
}

// endregion: claim

// region: store

// insertResultSQL is the Week 8 insert of one check_results row.
const insertResultSQL = `
INSERT INTO check_results
    (monitor_id, checked_at, up, status_code, dns_ms, connect_ms, tls_ms, first_byte_ms, total_ms, error)
VALUES ($1, $2, $3, NULLIF($4, 0), $5, $6, $7, $8, $9, NULLIF($10, ''))`

// runAndStore runs the check, then inserts its result and deletes the job
// inside tx. A panic in check becomes an error, which keeps one bad job
// from stopping the worker.
func runAndStore(ctx context.Context, tx pgx.Tx, job Job, check CheckFunc) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("job %d: check panicked: %v", job.ID, p)
		}
	}()
	r := check(ctx, job)
	if ctx.Err() != nil {
		return fmt.Errorf("job %d: %w", job.ID, ctx.Err())
	}
	if _, err := tx.Exec(ctx, insertResultSQL, job.MonitorID, r.CheckedAt, r.Up, r.StatusCode,
		r.DNSMs, r.ConnectMs, r.TLSMs, r.FirstByteMs, r.TotalMs, r.Error); err != nil {
		return fmt.Errorf("job %d: insert result: %w", job.ID, err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM check_jobs WHERE id = $1`, job.ID); err != nil {
		return fmt.Errorf("job %d: delete: %w", job.ID, err)
	}
	return nil
}

// endregion: store

// region: retry

// MaxAttempts is how many failed tries a job gets before Work drops it.
const MaxAttempts = 3

// retry counts a failed try of job outside the rolled-back transaction.
// It moves run_at 30 seconds further out for each failure, and drops the
// job after MaxAttempts failures. The scheduler queues a fresh job for the
// monitor once its interval has passed.
func retry(ctx context.Context, db *pgxpool.Pool, job Job) error {
	if job.Attempts+1 >= MaxAttempts {
		if _, err := db.Exec(ctx, `DELETE FROM check_jobs WHERE id = $1`, job.ID); err != nil {
			return err
		}
		return fmt.Errorf("job %d: dropped after %d attempts", job.ID, MaxAttempts)
	}
	_, err := db.Exec(ctx, `
		UPDATE check_jobs
		SET attempts = attempts + 1,
		    run_at = now() + make_interval(secs => 30 * (attempts + 1))
		WHERE id = $1`, job.ID)
	return err
}

// endregion: retry
