// region: builders
import type { Incident, Monitor, Result } from './api'

// Builders for test data: each returns a valid value, and a test names
// only the fields it cares about.

export function monitor(over: Partial<Monitor> = {}): Monitor {
  return { id: 1, url: 'https://example.com', interval_seconds: 60, created_at: '2026-10-11T09:00:00Z', ...over }
}

export function result(over: Partial<Result> = {}): Result {
  return {
    monitor_id: 1,
    checked_at: '2026-10-11T14:00:00Z',
    up: true,
    status_code: 200,
    dns_ms: 12,
    connect_ms: 11,
    tls_ms: 13,
    first_byte_ms: 52,
    total_ms: 53,
    error: null,
    ...over,
  }
}

// down is a failed check: no response, no total time, an error.
export function down(over: Partial<Result> = {}): Result {
  return result({
    up: false,
    status_code: null,
    first_byte_ms: null,
    total_ms: null,
    error: 'context deadline exceeded',
    ...over,
  })
}

export function incident(over: Partial<Incident> = {}): Incident {
  return { id: 1, monitor_id: 1, started_at: '2026-10-11T13:58:00Z', resolved_at: null, cause: 'context deadline exceeded', ...over }
}
// endregion: builders
