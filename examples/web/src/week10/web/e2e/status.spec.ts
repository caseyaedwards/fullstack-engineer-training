import { expect, test, type Page } from '@playwright/test'

// region: mock
// The api, as plain data. page.route answers the requests of the page from
// it, which means no api, worker or Postgres runs during these tests.
type Api = { monitors: object[]; results: Record<string, object[]>; incidents: object[] }

const up = (at: string, ms: number) => ({
  monitor_id: 1, checked_at: at, up: true, status_code: 200,
  dns_ms: 12, connect_ms: 11, tls_ms: 13, first_byte_ms: ms - 1, total_ms: ms, error: null,
})
const down = (at: string) => ({
  monitor_id: 1, checked_at: at, up: false, status_code: null,
  dns_ms: 12, connect_ms: null, tls_ms: null, first_byte_ms: null, total_ms: null, error: 'context deadline exceeded',
})

async function mockApi(page: Page, api: Api) {
  // Any /api/ request that no route below answers fails the test.
  await page.route('**/api/**', (route) => route.fulfill({ status: 599, body: `unmocked ${route.request().url()}` }))
  // Later routes win over earlier ones that match the same URL.
  await page.route('**/api/monitors', (route) => route.fulfill({ json: api.monitors }))
  await page.route('**/api/monitors/*/results?limit=50', (route) => {
    const id = new URL(route.request().url()).pathname.split('/')[3]
    return route.fulfill({ json: api.results[id] ?? [] })
  })
  await page.route('**/api/incidents?open=true', (route) => route.fulfill({ json: api.incidents }))
}
// endregion: mock

// region: flow
test('a site goes down: the page shows it at the next refresh', async ({ page }) => {
  // The page clock starts at 14:03 and moves only when the test says so.
  await page.clock.install({ time: new Date('2026-10-11T14:03:00Z') })
  const api: Api = {
    monitors: [{ id: 1, url: 'https://example.com', interval_seconds: 60, created_at: '2026-10-11T09:00:00Z' }],
    results: { 1: [up('2026-10-11T14:02:00Z', 50), up('2026-10-11T14:01:00Z', 200), up('2026-10-11T14:00:00Z', 100)] },
    incidents: [],
  }
  await mockApi(page, api)

  await page.goto('/')
  const row = page.getByRole('listitem', { name: 'https://example.com' })
  await expect(row.getByText('Up', { exact: true })).toBeVisible()
  await expect(row.getByRole('img')).toHaveAccessibleName('Total time of the last 3 checks: 50 to 200 ms, 0 failed')
  await expect(page.getByRole('region', { name: 'Open incidents' })).toHaveCount(0)

  // The worker records three failures; the alerter opens an incident.
  api.results[1] = [down('2026-10-11T14:05:00Z'), down('2026-10-11T14:04:00Z'), down('2026-10-11T14:03:00Z'), ...api.results[1]]
  api.incidents = [{ id: 9, monitor_id: 1, started_at: '2026-10-11T14:05:00Z', resolved_at: null, cause: 'context deadline exceeded' }]
  await page.clock.runFor(30_000)

  await expect(row.getByText('Down', { exact: true })).toBeVisible()
  await expect(row.getByRole('img')).toHaveAccessibleName('Total time of the last 6 checks: 50 to 200 ms, 3 failed')
  await expect(page.getByRole('region', { name: 'Open incidents' })).toContainText(
    'https://example.com is down since',
  )
})
// endregion: flow

// region: error
test('the api is down: the page says so', async ({ page }) => {
  await page.route('**/api/**', (route) => route.fulfill({ status: 502, body: 'Bad Gateway' }))
  await page.goto('/')
  await expect(page.getByRole('alert')).toHaveText('Cannot load monitors: GET /api/monitors: status 502')
})
// endregion: error
