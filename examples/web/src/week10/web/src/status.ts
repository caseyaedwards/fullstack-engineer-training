import type { Result } from './api'

// region: status
export type Status = 'up' | 'down' | 'unknown'

// currentStatus reads the newest result. The api sends results newest
// first, which puts it at index 0. A monitor with no results yet is
// unknown: the worker has not checked it.
export function currentStatus(results: Result[]): Status {
  if (results.length === 0) return 'unknown'
  return results[0].up ? 'up' : 'down'
}
// endregion: status

// region: plot
// Plot is what the latency chart draws: a dot for every check that
// succeeded, an SVG path that joins neighboring dots and breaks at every
// failed check, and the x position of each failed check. minMs and maxMs
// are null when no check succeeded.
export type Plot = {
  points: { x: number; y: number }[]
  path: string
  failures: number[]
  minMs: number | null
  maxMs: number | null
}

const round = (n: number) => Math.round(n * 10) / 10

// plot maps results, newest first as the api sends them, onto a box of
// width by height. Time runs left to right, from the oldest check to the
// newest, which means a pause in checking leaves a gap. SVG puts y = 0 at
// the top: a slow check gets a small y and sits high in the box.
export function plot(results: Result[], width: number, height: number): Plot {
  const checks = [...results].reverse()
  const times = checks.map((r) => Date.parse(r.checked_at))
  const first = times[0]
  const last = times[times.length - 1]
  const x = (t: number) => (last === first ? width / 2 : ((t - first) / (last - first)) * width)

  const ok = checks.filter((r) => r.up && r.total_ms !== null).map((r) => r.total_ms as number)
  const maxMs = ok.length ? Math.max(...ok) : null
  const y = (ms: number) => height - (ms / (maxMs || 1)) * height

  const points: { x: number; y: number }[] = []
  const parts: string[] = []
  const failures: number[] = []
  let pen = 'M' // M moves the pen, L draws a line to the point
  checks.forEach((r, i) => {
    if (!r.up || r.total_ms === null) {
      failures.push(round(x(times[i])))
      pen = 'M'
      return
    }
    const point = { x: round(x(times[i])), y: round(y(r.total_ms)) }
    points.push(point)
    parts.push(`${pen}${point.x},${point.y}`)
    pen = 'L'
  })
  return { points, path: parts.join(' '), failures, minMs: ok.length ? Math.min(...ok) : null, maxMs }
}
// endregion: plot
