// The typed client for the api. The site tests this file as the reader's
// web/src/api.ts at the end of Week 10.

// region: types
// The JSON bodies the api sends. Each type mirrors a Go struct: Monitor
// from Week 7, Result from Week 8, Incident from Week 9. A Go pointer field
// such as *int arrives as a number or null. Times arrive as RFC 3339 text,
// such as "2026-10-11T14:02:07Z".
export type Monitor = {
  id: number
  url: string
  interval_seconds: number
  created_at: string
}

export type Result = {
  monitor_id: number
  checked_at: string
  up: boolean
  status_code: number | null
  dns_ms: number | null
  connect_ms: number | null
  tls_ms: number | null
  first_byte_ms: number | null
  total_ms: number | null
  error: string | null
}

export type Incident = {
  id: number
  monitor_id: number
  started_at: string
  resolved_at: string | null
  cause: string
}

// The body of GET /healthz, from Week 6.
export type Health = { status: string }
// endregion: types

// region: error
// ApiError is a response with a status code outside 200 to 299. The api
// sends {"error": "..."} with every error; a proxy in front of it, such as
// Vite or nginx, answers 502 with a body of its own, which has no such field.
export class ApiError extends Error {
  readonly status: number

  constructor(path: string, status: number, detail: string) {
    super(`GET ${path}: status ${status}${detail ? `: ${detail}` : ''}`)
    this.name = 'ApiError'
    this.status = status
  }
}

// getJSON sends GET path and returns the parsed body. The signal cancels
// the request: the page passes one when it stops caring about the answer.
async function getJSON<T>(fetchFn: typeof fetch, path: string, signal?: AbortSignal): Promise<T> {
  const res = await fetchFn(path, { signal, headers: { Accept: 'application/json' } })
  if (!res.ok) {
    let detail = ''
    try {
      const body: unknown = await res.json()
      if (body && typeof body === 'object' && 'error' in body && typeof body.error === 'string') {
        detail = body.error
      }
    } catch {
      // Not JSON: keep the status code alone.
    }
    throw new ApiError(path, res.status, detail)
  }
  return (await res.json()) as T
}
// endregion: error

// region: client
// Client is everything the status page asks the api. Components receive a
// Client instead of calling fetch: a test hands them a fake one.
export type Client = {
  listMonitors(signal?: AbortSignal): Promise<Monitor[]>
  listResults(monitorID: number, limit: number, signal?: AbortSignal): Promise<Result[]>
  listOpenIncidents(signal?: AbortSignal): Promise<Incident[]>
}

// newClient returns a Client that sends its requests with fetchFn. The
// paths have no host: the browser sends them to the origin of the page,
// and the Vite proxy or nginx passes them on to the api.
export function newClient(fetchFn: typeof fetch = fetch): Client {
  return {
    listMonitors: (signal) => getJSON(fetchFn, '/api/monitors', signal),
    listResults: (monitorID, limit, signal) =>
      getJSON(fetchFn, `/api/monitors/${monitorID}/results?limit=${limit}`, signal),
    listOpenIncidents: (signal) => getJSON(fetchFn, '/api/incidents?open=true', signal),
  }
}

// getHealth is the Week 6 call, now built on getJSON.
export function getHealth(fetchFn: typeof fetch = fetch): Promise<Health> {
  return getJSON(fetchFn, '/healthz')
}
// endregion: client
