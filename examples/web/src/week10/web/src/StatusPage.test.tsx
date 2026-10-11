import { act, render, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { ApiError, type Client, type Incident, type Monitor, type Result } from './api'
import { StatusPage } from './StatusPage'
import { down, incident, monitor, result } from './testdata'

// region: fake-client
// fakeClient answers from plain data instead of the network, and records
// every call with its signal. A test changes the data between refreshes.
function fakeClient(data: { monitors: Monitor[]; results?: Record<number, Result[]>; incidents?: Incident[] }) {
  const calls: { what: string; signal?: AbortSignal }[] = []
  const client: Client = {
    listMonitors: async (signal) => {
      calls.push({ what: 'monitors', signal })
      return data.monitors
    },
    listResults: async (id, limit, signal) => {
      calls.push({ what: `results ${id} limit=${limit}`, signal })
      return data.results?.[id] ?? []
    },
    listOpenIncidents: async (signal) => {
      calls.push({ what: 'incidents', signal })
      return data.incidents ?? []
    },
  }
  return { client, calls, data }
}

// row finds the list item of one monitor by its accessible name, the URL.
const row = (url: string) => screen.getByRole('listitem', { name: url })
// endregion: fake-client

afterEach(() => {
  vi.useRealTimers()
})

// region: render-tests
describe('StatusPage', () => {
  test('shows each monitor with the status of its newest result and a chart', async () => {
    const { client, calls } = fakeClient({
      monitors: [monitor({ id: 1, url: 'https://example.com' }), monitor({ id: 2, url: 'https://example.org' })],
      results: {
        1: [result({ total_ms: 53 }), down()],
        2: [down({ monitor_id: 2 }), result({ monitor_id: 2 })],
      },
    })
    render(<StatusPage client={client} />)

    // findBy waits until the element appears: the data arrives after the first render.
    expect(await within(await screen.findByRole('listitem', { name: 'https://example.com' })).findByText('Up')).toBeVisible()
    expect(await within(row('https://example.org')).findByText('Down')).toBeVisible()
    expect(within(row('https://example.com')).getByText(/^Checked .*: 53 ms, status 200$/)).toBeVisible()
    expect(within(row('https://example.org')).getByText(/^Checked .*: context deadline exceeded$/)).toBeVisible()
    expect(within(row('https://example.com')).getByRole('img')).toHaveAccessibleName(
      'Total time of the last 2 checks: 53 to 53 ms, 1 failed',
    )
    expect(calls.map((c) => c.what)).toContain('results 1 limit=50')
  })

  test('lists open incidents with the URL of their monitor', async () => {
    const { client } = fakeClient({
      monitors: [monitor({ id: 3, url: 'https://example.net' })],
      incidents: [incident({ monitor_id: 3, cause: 'status 503' })],
    })
    render(<StatusPage client={client} />)

    const section = await screen.findByRole('region', { name: 'Open incidents' })
    expect(section).toHaveTextContent(/https:\/\/example\.net is down since .*: status 503/)
    expect(within(section).getByText(/:/, { selector: 'time' })).toHaveAttribute('datetime', '2026-10-11T13:58:00.000Z')
  })

  test('says so when there are no monitors and no incidents', async () => {
    const { client } = fakeClient({ monitors: [] })
    render(<StatusPage client={client} />)

    expect(await screen.findByText('No monitors yet. Add one with POST /api/monitors.')).toBeVisible()
    expect(screen.queryByRole('region', { name: 'Open incidents' })).not.toBeInTheDocument()
  })

  test('shows a monitor with no results as not checked yet, without a chart', async () => {
    const { client } = fakeClient({ monitors: [monitor()] })
    render(<StatusPage client={client} />)

    expect(await within(await screen.findByRole('listitem', { name: 'https://example.com' })).findByText('No checks yet')).toBeVisible()
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
  })
})
// endregion: render-tests

// region: failure-tests
describe('StatusPage when the api fails', () => {
  test('shows the error when the first request fails', async () => {
    const { client } = fakeClient({ monitors: [] })
    client.listMonitors = async () => {
      throw new ApiError('/api/monitors', 502, '')
    }
    render(<StatusPage client={client} />)

    expect(await screen.findByRole('alert')).toHaveTextContent('Cannot load monitors: GET /api/monitors: status 502')
  })

  test('says so when it cannot load the incidents, instead of showing none', async () => {
    const { client } = fakeClient({ monitors: [monitor()] })
    client.listOpenIncidents = async () => {
      throw new ApiError('/api/incidents?open=true', 500, 'internal error')
    }
    render(<StatusPage client={client} />)

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Cannot load incidents: GET /api/incidents?open=true: status 500: internal error',
    )
  })

  test('keeps the last data and warns when a refresh fails', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    const { client, data } = fakeClient({ monitors: [monitor()], results: { 1: [result()] } })
    render(<StatusPage client={client} refreshMs={30_000} />)
    expect(await within(await screen.findByRole('listitem', { name: 'https://example.com' })).findByText('Up')).toBeVisible()

    // The next refresh: the monitor list fails, the results now say down.
    client.listMonitors = async () => {
      throw new ApiError('/api/monitors', 502, '')
    }
    data.results = { 1: [down(), result()] }
    await act(() => vi.advanceTimersByTimeAsync(30_000))

    expect(screen.getByRole('alert')).toHaveTextContent(/^Showing data from .*\. The last refresh failed: GET \/api\/monitors: status 502$/)
    expect(within(row('https://example.com')).getByText('Down')).toBeVisible()
  })

  test('cancels its requests when it leaves the page', async () => {
    const { client, calls } = fakeClient({ monitors: [monitor()] })
    const { unmount } = render(<StatusPage client={client} />)
    await screen.findByRole('listitem', { name: 'https://example.com' })

    unmount()
    expect(calls.length).toBeGreaterThanOrEqual(3)
    for (const c of calls) expect(c.signal?.aborted).toBe(true)
  })

  test('stops asking when it leaves the page, even when an answer arrives late', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    const { client } = fakeClient({ monitors: [] })
    let answer: (m: Monitor[]) => void = () => {}
    let asked = 0
    client.listMonitors = () => {
      asked++
      return new Promise((resolve) => (answer = resolve)) // a slow api that ignores the signal
    }
    const { unmount } = render(<StatusPage client={client} />)

    unmount()
    answer([monitor()])
    await vi.advanceTimersByTimeAsync(60_000)
    expect(asked).toBe(1)
  })
})
// endregion: failure-tests
