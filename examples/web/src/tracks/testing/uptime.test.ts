// region: tests
import { describe, expect, it } from 'vitest'
import { formatUptime, uptimePercent } from './uptime'

// checks builds a list of results: ok successes, then failures up to total.
function checks(ok: number, total: number): { ok: boolean }[] {
  return Array.from({ length: total }, (_, i) => ({ ok: i < ok }))
}

describe('uptimePercent and formatUptime', () => {
  it.each([
    { ok: 0, total: 0, percent: null, text: 'no data' },
    { ok: 3, total: 4, percent: 75, text: '75.00%' },
    { ok: 999, total: 1000, percent: 99.9, text: '99.90%' },
    { ok: 4, total: 4, percent: 100, text: '100.00%' },
    { ok: 0, total: 4, percent: 0, text: '0.00%' },
    { ok: 2, total: 3, percent: 66.66666666666667, text: '66.67%' },
    // 29 / 100 * 100 is 28.999999999999996 in floating point.
    { ok: 29, total: 100, percent: 29, text: '29.00%' },
    // 99.999 rounds to 100.00, which claims a perfect record.
    { ok: 99_999, total: 100_000, percent: 99.999, text: '99.99%' },
  ])('$ok of $total checks up gives $text', ({ ok, total, percent, text }) => {
    const p = uptimePercent(checks(ok, total))
    expect(p).toBe(percent)
    expect(formatUptime(p)).toBe(text)
  })
})
// endregion: tests
