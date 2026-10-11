// region: uptime
// uptimePercent returns the share of results that are ok, from 0 to 100,
// or null when there are no results: no data is not the same as 0% up.
export function uptimePercent(results: { ok: boolean }[]): number | null {
  if (results.length === 0) return null
  const up = results.filter((r) => r.ok).length
  // Multiply first: one rounding step. up / length * 100 rounds twice,
  // and 29 of 100 comes out as 28.999999999999996.
  return (up * 100) / results.length
}

// formatUptime shows a percentage with two decimals, as the status page
// does, or 'no data' for null.
export function formatUptime(p: number | null): string {
  if (p === null) return 'no data'
  const text = p.toFixed(2)
  // 99.999 rounds to 100.00, a perfect record that does not exist.
  return `${text === '100.00' && p < 100 ? '99.99' : text}%`
}
// endregion: uptime
