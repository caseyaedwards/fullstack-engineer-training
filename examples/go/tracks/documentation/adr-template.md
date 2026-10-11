# Architecture Decision Records

Each record lives in `docs/adr/` as one Markdown file, numbered in order:
`0001-record-architecture-decisions.md`, `0002-keep-the-job-queue-in-postgres.md`.
Copy the template for a new decision. Never edit the decision of an accepted
record: write a new record that supersedes it, and change only the status line
of the old one.

<!-- region: template -->
# NNNN: Title in the Imperative, Such as Keep the Job Queue in Postgres

## Status

Proposed | Accepted | Superseded by NNNN

Date: Month D, YYYY

## Context

The forces at play: what the system needs, what limits it, and which options
exist. Write facts and numbers, not the decision.

## Decision

We will ... One paragraph, in the active voice.

## Consequences

What becomes easier, what becomes harder, and what to watch for that tells
you to revisit this decision.
<!-- endregion: template -->

<!-- region: record -->
# 0002: Keep the Job Queue in Postgres

## Status

Accepted

Date: October 11, 2026

## Context

From Week 9, a scheduler decides which monitors are due and workers run the
checks. Two workers must never run the same job, and a job must survive a
worker that crashes in the middle of a check. The monitor checks 300 sites
every 60 seconds: five jobs a second. Postgres already stores the monitors
and the results, and `pg_dump` backs it up. The options are a broker, such as
NATS, RabbitMQ or Redis Streams, or a `jobs` table that workers claim with
`SELECT ... FOR UPDATE SKIP LOCKED`.

## Decision

We will keep the queue in a `jobs` table in the existing Postgres. A worker
claims a job, runs the check, and inserts the result and deletes the job in
one transaction.

## Consequences

- One database to run, back up and restore. The queue needs no new server.
- The result and the end of the job commit together: a result is never stored
  twice, and a job is never lost.
- A job runs at least once. A worker that dies before its commit leaves the
  job to the next worker, and every job must be safe to run twice.
- Workers poll the table, and each check in flight holds a connection.
- Revisit this decision when the queue passes 1,000 jobs a second, or when a
  second kind of consumer needs the same events.
<!-- endregion: record -->
