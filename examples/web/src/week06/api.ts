// region: api
// The JSON body of GET /healthz, as cmd/api writes it.
export type Health = { status: string };

// getHealth asks the API for its health. The path is relative: the browser
// sends it to the origin that served the page, and in development the Vite
// dev server passes it on to the api. Tests pass in a fake fetch.
export async function getHealth(fetchFn: typeof fetch = fetch): Promise<Health> {
  const res = await fetchFn('/healthz');
  if (!res.ok) {
    throw new Error(`GET /healthz: status ${res.status}`);
  }
  return (await res.json()) as Health;
}
// endregion: api
