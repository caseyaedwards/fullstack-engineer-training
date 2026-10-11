import { describe, expect, test } from 'vitest';
import { getHealth } from './api';

// region: test
// fakeFetch answers every request with one canned response and records the URL.
function fakeFetch(status: number, body: unknown) {
  const urls: string[] = [];
  const fn = (async (input: RequestInfo | URL) => {
    urls.push(String(input));
    return new Response(JSON.stringify(body), {
      status,
      headers: { 'Content-Type': 'application/json' },
    });
  }) as typeof fetch;
  return { fn, urls };
}

describe('getHealth', () => {
  test('asks /healthz on the page origin and returns the parsed body', async () => {
    const { fn, urls } = fakeFetch(200, { status: 'ok' });
    await expect(getHealth(fn)).resolves.toEqual({ status: 'ok' });
    expect(urls).toEqual(['/healthz']);
  });

  test('throws with the status when the api answers an error', async () => {
    const { fn } = fakeFetch(503, { status: 'starting' });
    await expect(getHealth(fn)).rejects.toThrow('GET /healthz: status 503');
  });
});
// endregion: test
