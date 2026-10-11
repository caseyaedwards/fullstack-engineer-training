import { useCallback } from 'react'
import type { Client, Incident, Monitor } from './api'
import { LatencyChart } from './LatencyChart'
import { currentStatus } from './status'
import { usePolling } from './usePolling'

// How many results each chart shows, and how often the page asks again.
export const RESULT_LIMIT = 50
export const REFRESH_MS = 30_000

// region: page
// StatusPage is the whole page: the open incidents, then one row per
// monitor. It asks the api through client, which a test replaces.
export function StatusPage({ client, refreshMs = REFRESH_MS }: { client: Client; refreshMs?: number }) {
  // useCallback keeps each function the same between renders. A new
  // function on every render restarts usePolling on every render.
  const loadMonitors = useCallback((signal: AbortSignal) => client.listMonitors(signal), [client])
  const loadIncidents = useCallback((signal: AbortSignal) => client.listOpenIncidents(signal), [client])
  const monitors = usePolling(loadMonitors, refreshMs)
  const incidents = usePolling(loadIncidents, refreshMs)

  if (!monitors.data) {
    return (
      <main>
        <h1>Status</h1>
        {monitors.error ? (
          <p role="alert">Cannot load monitors: {monitors.error.message}</p>
        ) : (
          <p>Loading monitors…</p>
        )}
      </main>
    )
  }

  return (
    <main>
      <h1>Status</h1>
      {monitors.error && (
        <p role="alert">
          Showing data from <Time date={monitors.updatedAt} />. The last refresh failed: {monitors.error.message}
        </p>
      )}
      {incidents.error && <p role="alert">Cannot load incidents: {incidents.error.message}</p>}
      <IncidentList incidents={incidents.data ?? []} monitors={monitors.data} />
      {monitors.data.length === 0 ? (
        <p>No monitors yet. Add one with POST /api/monitors.</p>
      ) : (
        <ul className="monitors">
          {monitors.data.map((m) => (
            <MonitorRow key={m.id} monitor={m} client={client} refreshMs={refreshMs} />
          ))}
        </ul>
      )}
      <p className="updated">
        Updated <Time date={monitors.updatedAt} />
      </p>
    </main>
  )
}
// endregion: page

// region: row
const LABELS = { up: 'Up', down: 'Down', unknown: 'No checks yet' }

// MonitorRow asks for the recent results of one monitor and shows its
// status, its newest check and its chart.
function MonitorRow({ monitor, client, refreshMs }: { monitor: Monitor; client: Client; refreshMs: number }) {
  const load = useCallback(
    (signal: AbortSignal) => client.listResults(monitor.id, RESULT_LIMIT, signal),
    [client, monitor.id],
  )
  const results = usePolling(load, refreshMs)
  const list = results.data ?? []
  const status = currentStatus(list)
  const newest = list[0]

  return (
    <li aria-labelledby={`monitor-${monitor.id}`}>
      <h2 id={`monitor-${monitor.id}`}>{monitor.url}</h2>
      <span className={`badge ${status}`}>{LABELS[status]}</span>
      {newest && (
        <p>
          Checked <Time date={new Date(newest.checked_at)} />:{' '}
          {newest.up ? `${newest.total_ms} ms, status ${newest.status_code}` : newest.error}
        </p>
      )}
      {results.error && <p role="alert">Cannot load results: {results.error.message}</p>}
      {list.length > 0 && <LatencyChart results={list} />}
    </li>
  )
}
// endregion: row

// region: incidents
// IncidentList shows the open incidents, with the URL of each monitor.
function IncidentList({ incidents, monitors }: { incidents: Incident[]; monitors: Monitor[] }) {
  if (incidents.length === 0) return null
  const urls = new Map(monitors.map((m) => [m.id, m.url]))
  return (
    <section aria-labelledby="incidents">
      <h2 id="incidents">Open incidents</h2>
      <ul>
        {incidents.map((i) => (
          <li key={i.id}>
            {urls.get(i.monitor_id) ?? `monitor ${i.monitor_id}`} is down since{' '}
            <Time date={new Date(i.started_at)} />: {i.cause ?? 'no cause recorded'}
          </li>
        ))}
      </ul>
    </section>
  )
}

// Time shows a moment in the local time of the browser. The dateTime
// attribute keeps the exact value for tests and for screen readers.
function Time({ date }: { date?: Date }) {
  if (!date) return null
  return <time dateTime={date.toISOString()}>{date.toLocaleTimeString()}</time>
}
// endregion: incidents
