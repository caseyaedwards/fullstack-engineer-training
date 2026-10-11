import { describe, expect, test } from 'vitest'
import { currentStatus, plot } from './status'
import { down, result } from './testdata'

describe('currentStatus', () => {
  test('reads the newest result, which comes first', () => {
    expect(currentStatus([result(), down()])).toBe('up')
    expect(currentStatus([down(), result()])).toBe('down')
  })

  test('is unknown before the first check', () => {
    expect(currentStatus([])).toBe('unknown')
  })
})

// region: plot-test
describe('plot', () => {
  // Four checks a minute apart, newest first, as the api sends them.
  const results = [
    result({ checked_at: '2026-10-11T14:03:00Z', total_ms: 50 }),
    down({ checked_at: '2026-10-11T14:02:00Z' }),
    result({ checked_at: '2026-10-11T14:01:00Z', total_ms: 200 }),
    result({ checked_at: '2026-10-11T14:00:00Z', total_ms: 100 }),
  ]

  test('puts the oldest check on the left and the slowest at the top', () => {
    const p = plot(results, 300, 60)
    expect(p.points).toEqual([
      { x: 0, y: 30 }, // 14:00, 100 ms: half of the slowest
      { x: 100, y: 0 }, // 14:01, 200 ms: the slowest, at the top
      { x: 300, y: 45 }, // 14:03, 50 ms
    ])
    expect(p.minMs).toBe(50)
    expect(p.maxMs).toBe(200)
  })

  test('breaks the line at a failed check and marks where it failed', () => {
    const p = plot(results, 300, 60)
    expect(p.path).toBe('M0,30 L100,0 M300,45')
    expect(p.failures).toEqual([200])
  })

  test('puts a single check in the middle', () => {
    expect(plot([result({ total_ms: 80 })], 300, 60).points).toEqual([{ x: 150, y: 0 }])
  })

  test('has no line and no range when every check failed', () => {
    const p = plot([down(), down({ checked_at: '2026-10-11T13:59:00Z' })], 300, 60)
    expect(p).toEqual({ points: [], path: '', failures: [0, 300], minMs: null, maxMs: null })
  })
})
// endregion: plot-test
