import { describe, expect, test } from 'vitest'
import { AuthError, currentUser, login, logout } from './auth'

// fakeFetch answers every request with one canned response, and records
// each request.
function fakeFetch(status: number, body: string | null, headers: Record<string, string> = {}) {
  const calls: { url: string; init?: RequestInit }[] = []
  const fn = (async (input: RequestInfo | URL, init?: RequestInit) => {
    calls.push({ url: String(input), init })
    return new Response(body, { status, headers: { 'Content-Type': 'application/json', ...headers } })
  }) as typeof fetch
  return { fn, calls }
}

const you = { id: 1, email: 'you@example.com', created_at: '2026-10-11T09:00:00Z' }

// region: tests
describe('login', () => {
  test('POSTs the email and password as JSON and returns the user', async () => {
    const { fn, calls } = fakeFetch(200, JSON.stringify(you))
    await expect(login('you@example.com', 'correct horse battery', fn)).resolves.toEqual(you)
    expect(calls).toHaveLength(1)
    expect(calls[0].url).toBe('/api/login') // nothing secret in the URL
    expect(calls[0].init?.method).toBe('POST')
    expect(new Headers(calls[0].init?.headers).get('Content-Type')).toBe('application/json')
    expect(JSON.parse(String(calls[0].init?.body))).toEqual({ email: 'you@example.com', password: 'correct horse battery' })
  })

  test('turns a 401 into an AuthError with the api message', async () => {
    const { fn } = fakeFetch(401, '{"error":"wrong email or password"}\n')
    const err = await login('you@example.com', 'wrong', fn).catch((e: unknown) => e)
    expect(err).toBeInstanceOf(AuthError)
    expect(err).toMatchObject({ status: 401, message: 'wrong email or password', retryAfter: null })
  })

  test('reads Retry-After from a 429', async () => {
    const { fn } = fakeFetch(429, '{"error":"too many login attempts; try again in 90 seconds"}\n', { 'Retry-After': '90' })
    await expect(login('you@example.com', 'x', fn)).rejects.toMatchObject({ status: 429, retryAfter: 90 })
  })
})

describe('currentUser', () => {
  test('returns the user of the session', async () => {
    const { fn, calls } = fakeFetch(200, JSON.stringify(you))
    await expect(currentUser(fn)).resolves.toEqual(you)
    expect(calls[0].url).toBe('/api/me')
  })

  test('returns null when there is no session', async () => {
    const { fn } = fakeFetch(401, '{"error":"login required"}\n')
    await expect(currentUser(fn)).resolves.toBeNull()
  })

  test('throws on any other failure', async () => {
    const { fn } = fakeFetch(502, '<html>Bad Gateway</html>', { 'Content-Type': 'text/html' })
    await expect(currentUser(fn)).rejects.toMatchObject({ status: 502, message: 'status 502' })
  })
})

describe('logout', () => {
  test('POSTs to /api/logout', async () => {
    const { fn, calls } = fakeFetch(204, null)
    await expect(logout(fn)).resolves.toBeUndefined()
    expect(calls[0]).toMatchObject({ url: '/api/logout', init: { method: 'POST' } })
  })
})
// endregion: tests
