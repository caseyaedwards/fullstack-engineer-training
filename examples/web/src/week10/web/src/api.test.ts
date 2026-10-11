import { describe, expect, test } from 'vitest'
import { ApiError, getHealth, newClient } from './api'

// region: fake
// fakeFetch answers every request with one canned response. It records the
// URL and the signal of each call, which lets a test check what the client asks.
function fakeFetch(status: number, body: string, contentType = 'application/json') {
  const calls: { url: string; signal?: AbortSignal | null }[] = []
  const fn = (async (input: RequestInfo | URL, init?: RequestInit) => {
    calls.push({ url: String(input), signal: init?.signal })
    return new Response(body, { status, headers: { 'Content-Type': contentType } })
  }) as typeof fetch
  return { fn, calls }
}
// endregion: fake

// region: tests
describe('newClient', () => {
  test('asks each route on the page origin and returns the parsed body', async () => {
    const monitors = [{ id: 1, url: 'https://example.com', interval_seconds: 60, created_at: '2026-10-11T09:00:00Z' }]
    const { fn, calls } = fakeFetch(200, JSON.stringify(monitors))
    const client = newClient(fn)

    await expect(client.listMonitors()).resolves.toEqual(monitors)
    await client.listResults(7, 50)
    await client.listOpenIncidents()
    expect(calls.map((c) => c.url)).toEqual([
      '/api/monitors',
      '/api/monitors/7/results?limit=50',
      '/api/incidents?open=true',
    ])
  })

  test('passes the abort signal on to fetch', async () => {
    const { fn, calls } = fakeFetch(200, '[]')
    const controller = new AbortController()
    await newClient(fn).listResults(1, 50, controller.signal)
    expect(calls[0].signal).toBe(controller.signal)
  })

  test('turns an api error body into an ApiError with its message', async () => {
    const { fn } = fakeFetch(404, '{"error":"monitor 7 not found"}\n')
    const err = await newClient(fn).listResults(7, 50).catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect(err).toMatchObject({ status: 404, message: 'GET /api/monitors/7/results?limit=50: status 404: monitor 7 not found' })
  })

  test('keeps the status alone when a proxy answers with a body that is not JSON', async () => {
    const { fn } = fakeFetch(502, '<html>Bad Gateway</html>', 'text/html')
    await expect(newClient(fn).listMonitors()).rejects.toThrow(/^GET \/api\/monitors: status 502$/)
  })
})

describe('getHealth', () => {
  test('keeps the Week 6 behavior', async () => {
    await expect(getHealth(fakeFetch(200, '{"status":"ok"}').fn)).resolves.toEqual({ status: 'ok' })
    await expect(getHealth(fakeFetch(503, '{"status":"starting"}').fn)).rejects.toThrow('GET /healthz: status 503')
  })
})
// endregion: tests
