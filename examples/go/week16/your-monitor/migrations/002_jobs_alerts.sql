-- Week 9: the job queue and the incidents the alerter opens.

-- region: jobs
-- One row is one check that waits for a worker. A worker deletes the row
-- in the same transaction that stores the result.
CREATE TABLE check_jobs (
    id         bigserial PRIMARY KEY,
    monitor_id bigint NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
    run_at     timestamptz NOT NULL DEFAULT now(),
    attempts   int NOT NULL DEFAULT 0
);

-- Workers take the job with the oldest run_at first.
CREATE INDEX check_jobs_run_at ON check_jobs (run_at);
-- The scheduler looks up the waiting job of a monitor, and deleting a
-- monitor deletes its jobs: both search by monitor_id.
CREATE INDEX check_jobs_monitor ON check_jobs (monitor_id);

-- When the scheduler last queued a check for this monitor. NULL: never.
ALTER TABLE monitors ADD COLUMN last_enqueued_at timestamptz;
-- endregion: jobs

-- region: incidents
-- An incident is open while resolved_at is NULL.
CREATE TABLE incidents (
    id          bigserial PRIMARY KEY,
    monitor_id  bigint NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
    started_at  timestamptz NOT NULL,
    resolved_at timestamptz,
    cause       text
);

-- At most one open incident per monitor. A second alerter that tries to
-- open another one gets a conflict instead of a duplicate.
CREATE UNIQUE INDEX incidents_one_open ON incidents (monitor_id) WHERE resolved_at IS NULL;
-- endregion: incidents
