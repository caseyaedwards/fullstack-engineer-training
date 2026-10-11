-- contract stub: week 08
CREATE TABLE monitors (
    id               bigserial PRIMARY KEY,
    url              text NOT NULL UNIQUE,
    interval_seconds int NOT NULL CHECK (interval_seconds BETWEEN 30 AND 3600),
    created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE check_results (
    id            bigserial PRIMARY KEY,
    monitor_id    bigint NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
    checked_at    timestamptz NOT NULL,
    up            boolean NOT NULL,
    status_code   int,
    dns_ms        int,
    connect_ms    int,
    tls_ms        int,
    first_byte_ms int,
    total_ms      int,
    error         text
);

CREATE INDEX check_results_monitor_time ON check_results (monitor_id, checked_at DESC);
