-- region: init
-- 001_init.sql: the first schema of the uptime monitor. Migrate runs
-- this file once, inside a transaction, and records version 1 in
-- schema_migrations. Never edit it after it has run anywhere: add
-- 002_<name>.sql instead.

-- One row per URL to check.
create table monitors (
    id               bigserial primary key,
    url              text not null unique,
    interval_seconds int not null check (interval_seconds between 30 and 3600),
    created_at       timestamptz not null default now()
);

-- One row per check. A NULL timing means that step did not happen.
create table check_results (
    id            bigserial primary key,
    monitor_id    bigint not null references monitors (id) on delete cascade,
    checked_at    timestamptz not null,
    up            boolean not null,
    status_code   int,
    dns_ms        int,
    connect_ms    int,
    tls_ms        int,
    first_byte_ms int,
    total_ms      int,
    error         text
);

-- The history of one monitor, newest first: the query the API runs.
create index check_results_monitor_time on check_results (monitor_id, checked_at desc);
-- endregion: init
