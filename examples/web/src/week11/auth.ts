// The login calls of the status page: the reader's web/src/auth.ts in
// Week 11. The api sets and reads the session cookie; this code never
// sees it, because the cookie is HttpOnly.

// region: types
// User is the body of POST /api/login and GET /api/me.
export type User = {
  id: number
  email: string
  created_at: string
}

// AuthError is a login call that the api refused. retryAfter holds the
// seconds from the Retry-After header of a 429, and null otherwise.
export class AuthError extends Error {
  readonly status: number
  readonly retryAfter: number | null

  constructor(status: number, message: string, retryAfter: number | null) {
    super(message)
    this.name = 'AuthError'
    this.status = status
    this.retryAfter = retryAfter
  }
}

// fail turns a response outside 200 to 299 into an AuthError, with the
// message from the {"error": "..."} body when there is one.
async function fail(res: Response): Promise<never> {
  let message = `status ${res.status}`
  try {
    const body: unknown = await res.json()
    if (body && typeof body === 'object' && 'error' in body && typeof body.error === 'string') {
      message = body.error
    }
  } catch {
    // Not JSON: keep the status code alone.
  }
  const retry = res.headers.get('Retry-After')
  throw new AuthError(res.status, message, retry === null ? null : Number(retry))
}
// endregion: types

// region: calls
// The paths have no host, as in api.ts: the browser sends them to the
// origin of the page. fetch sends cookies with a request to the same
// origin, which is the default credentials: 'same-origin'.

// login sends the email and password in a JSON body, never in the URL,
// where server logs and the browser history keep them.
export async function login(email: string, password: string, fetchFn: typeof fetch = fetch): Promise<User> {
  const res = await fetchFn('/api/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
    body: JSON.stringify({ email, password }),
  })
  if (!res.ok) return fail(res)
  return (await res.json()) as User
}

// currentUser returns the logged-in user, or null when there is no
// session: a 401 here is an answer, not an error.
export async function currentUser(fetchFn: typeof fetch = fetch): Promise<User | null> {
  const res = await fetchFn('/api/me', { headers: { Accept: 'application/json' } })
  if (res.status === 401) return null
  if (!res.ok) return fail(res)
  return (await res.json()) as User
}

export async function logout(fetchFn: typeof fetch = fetch): Promise<void> {
  const res = await fetchFn('/api/logout', { method: 'POST' })
  if (!res.ok) return fail(res)
}
// endregion: calls
