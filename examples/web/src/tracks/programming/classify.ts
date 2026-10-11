// The TypeScript twin of examples/go/tracks/programming: the same classes,
// with Error.cause in place of %w wrapping.

// region: types
// ErrTimeout is one shared value that callers compare against, like the Go sentinel.
export const ErrTimeout = new Error('timeout')

// StatusError carries the HTTP status code of a response the check counts as a failure.
export class StatusError extends Error {
  readonly code: number

  constructor(code: number) {
    super(`HTTP status ${code}`)
    this.name = 'StatusError'
    this.code = code
  }
}
// endregion: types

// region: classify
// classify names the class of err: ok, timeout, http-4xx, http-5xx or other.
// The includes and find calls play the parts of errors.Is and errors.As.
export function classify(err: unknown): string {
  if (err == null) return 'ok'
  const chain = causes(err)
  if (chain.includes(ErrTimeout)) return 'timeout'
  const se = chain.find((e) => e instanceof StatusError)
  if (se instanceof StatusError) {
    if (se.code >= 500 && se.code <= 599) return 'http-5xx'
    if (se.code >= 400 && se.code <= 499) return 'http-4xx'
  }
  return 'other'
}

// causes lists err and every cause under it, and stops at a cycle.
function causes(err: unknown): unknown[] {
  const chain: unknown[] = []
  for (let e = err; e != null && !chain.includes(e); e = (e as { cause?: unknown }).cause) {
    chain.push(e)
  }
  return chain
}
// endregion: classify
