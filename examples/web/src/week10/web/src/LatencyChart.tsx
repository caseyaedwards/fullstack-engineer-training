import type { Result } from './api'
import { plot } from './status'

// region: chart
const WIDTH = 300
const HEIGHT = 60
const PAD = 4 // room for the dots at the edges

// LatencyChart draws the total time of each check as a line of dots, and
// each failed check as a red bar. The SVG scales to the width of its box.
// A screen reader cannot see the picture: aria-label says what it shows.
export function LatencyChart({ results }: { results: Result[] }) {
  const p = plot(results, WIDTH, HEIGHT)
  const failed = `${p.failures.length} failed`
  const label =
    p.maxMs === null
      ? `Total time of the last ${results.length} checks: all failed`
      : `Total time of the last ${results.length} checks: ${p.minMs} to ${p.maxMs} ms, ${failed}`

  return (
    <svg
      className="chart"
      viewBox={`${-PAD} ${-PAD} ${WIDTH + 2 * PAD} ${HEIGHT + 2 * PAD}`}
      role="img"
      aria-label={label}
    >
      {p.failures.map((x, i) => (
        <line key={i} className="failure" x1={x} x2={x} y1={0} y2={HEIGHT} />
      ))}
      <path className="line" d={p.path} />
      {p.points.map((pt, i) => (
        <circle key={i} className="point" cx={pt.x} cy={pt.y} r={2} />
      ))}
    </svg>
  )
}
// endregion: chart
