import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { base, root } from './helpers.mjs';

// Week 7 turns the api stub into a REST API for monitors, with an in-memory store,
// JSON validation, an OpenAPI file and httptest tests. These checks keep the page
// complete, and keep the routes, status codes and error bodies on the page in step
// with the tested example code and api/openapi.yaml.

const html = readFileSync(join(root, 'dist/weeks/07-backend-api/index.html'), 'utf8');
const figures = [...html.matchAll(/<figure class="dg"[^>]*>[\s\S]*?<\/figure>/g)].map((m) => m[0]);
// Expressive Code splits tokens into spans, so compare code on its text with tags removed.
const named = { amp: '&', lt: '<', gt: '>', quot: '"', apos: "'" };
const unescape = (s) =>
  s.replace(/&(?:#x([0-9a-f]+)|#(\d+)|(\w+));/gi, (m, hex, dec, name) =>
    hex ? String.fromCodePoint(parseInt(hex, 16)) : dec ? String.fromCodePoint(Number(dec)) : (named[name] ?? m));
const blocks = (lang) =>
  [...html.matchAll(new RegExp(`<pre data-language="${lang}">([\\s\\S]*?)</pre>`, 'g'))].map((m) =>
    unescape(m[1].replace(/<div class="ec-line[^"]*">/g, '\n').replace(/<[^>]+>/g, '')).replace(/^\n/, ''),
  );
const dir = join(root, 'examples/go/week07/your-monitor');
const example = (file) => readFileSync(join(dir, file), 'utf8');

test('week 7 has at least three inline-SVG diagrams, two with walkthrough steps', () => {
  assert.ok(figures.length >= 3, `expected >= 3 figure.dg, got ${figures.length}`);
  for (const f of figures) assert.match(f, /<svg[\s>]/, 'figure without inline <svg>');
  for (const id of ['request-flow', 'layers']) {
    assert.ok(figures.some((f) => f.includes(`id="${id}"`) && f.includes('dg-steps')), `diagram ${id} has no walkthrough`);
  }
});

test('week 7 shows the store, the routes, the JSON decoder and the tests', () => {
  const go = blocks('go').join('\n');
  assert.match(go, /type Store interface \{/);
  assert.match(go, /func Validate\(rawURL string, intervalSeconds int\) error/);
  assert.match(go, /func \(s \*MemStore\) Create\(ctx context\.Context, m Monitor\) \(Monitor, error\)/);
  for (const route of ['GET /healthz', 'GET /api/monitors', 'POST /api/monitors', 'GET /api/monitors/{id}', 'DELETE /api/monitors/{id}']) {
    assert.ok(go.includes(`"${route}":`), `route ${route} is not shown`);
  }
  assert.match(go, /http\.MaxBytesReader\(w, r\.Body, maxBodyBytes\)/);
  assert.match(go, /dec\.DisallowUnknownFields\(\)/);
  assert.match(go, /r\.PathValue\("id"\)/);
  assert.match(go, /w\.Header\(\)\.Set\("Location"/);
  assert.match(go, /func TestErrors\(t \*testing\.T\)/);
  assert.match(go, /func TestOpenAPIMatchesRoutes\(t \*testing\.T\)/);
  assert.match(go, /func LogRequests\(logger \*log\.Logger, next http\.Handler\) http\.Handler/);
});

// Parse the paths and methods of api/openapi.yaml, the way openapi_test.go does.
function documentedRoutes() {
  const routes = [];
  let path = '';
  for (const line of example('api/openapi.yaml').split('\n')) {
    const p = /^ {2}(\/\S*):$/.exec(line);
    const m = /^ {4}(get|put|post|patch|delete):$/.exec(line);
    if (p) path = p[1];
    else if (m && path) routes.push(`${m[1].toUpperCase()} ${path}`);
  }
  return routes.sort();
}

test('the routes table on the page matches api/openapi.yaml', () => {
  const table = [...html.matchAll(/<td><code dir="auto">((?:GET|POST|PUT|PATCH|DELETE) \/api\/[^<]*)<\/code><\/td>/g)].map((m) => m[1]);
  const documented = documentedRoutes().filter((r) => r.includes(' /api/'));
  assert.deepEqual([...new Set(table)].sort(), documented);
  const yaml = blocks('yaml').join('\n');
  assert.match(yaml, /^openapi: 3\.1\.\d+$/m, 'the page must show an OpenAPI 3.1 file');
});

test('every status code in the status table is documented in openapi.yaml or answered by the mux', () => {
  const spec = example('api/openapi.yaml');
  const start = html.indexOf('Your api answers it when');
  const table = html.slice(start, html.indexOf('</table>', start));
  const codes = [...table.matchAll(/<td><code dir="auto">(\d{3})<\/code><\/td>/g)].map((m) => m[1]);
  assert.ok(codes.length >= 10, `expected >= 10 status codes in the table, got ${codes.length}`);
  for (const code of codes) {
    if (code === '405') continue; // the mux answers 405 for every route; the spec does not list it per route
    assert.match(spec, new RegExp(`^ +"${code}":`, 'm'), `status ${code} is on the page but not in openapi.yaml`);
  }
});

test('the error bodies on the page are the messages the example code writes', () => {
  const txt = blocks('txt').join('\n');
  const errors = [...txt.matchAll(/^\{"error":"(.*)"\}$/gm)].map((m) => m[1].replace(/\\"/g, '"'));
  assert.ok(errors.length >= 4, `expected >= 4 error bodies in the page output, got ${errors.length}`);
  const tests = example('internal/api/api_test.go');
  for (const msg of errors) {
    // Each message on the page appears in the error table of api_test.go, or differs
    // only in the id or URL the request named.
    const pattern = msg.replace(/[.*+?^${}()|[\]\\]/g, '\\$&').replace(/\d+/g, '\\d+').replace(/https:\/\/example\\\.\w+/g, 'https://example\\.\\w+');
    const inTests = [...tests.matchAll(/, \d{3}, (?:"([^"]*)"|`([^`]*)`)\},/g)].some((m) => new RegExp(`^${pattern}$`).test(m[1] ?? m[2]));
    assert.ok(inTests, `error ${JSON.stringify(msg)} on the page has no matching row in TestErrors`);
  }
});

test('week 7 links to weeks 5, 6, 8 and 11 with the site base', () => {
  const hrefs = new Set([...html.matchAll(/href="([^"]+)"/g)].map((m) => m[1]));
  for (const slug of ['05-concurrency', '06-architecture-repo', '08-backend-postgres', '11-security-auth']) {
    const href = `${base}weeks/${slug}/`;
    assert.ok(hrefs.has(href), `no link to ${href}`);
    assert.ok(existsSync(join(root, 'dist/weeks', slug, 'index.html')), `${href} has no page`);
  }
});

test('week 7 has every standard box and no stub text', () => {
  for (const marker of ['callout callout-observe', 'callout callout-operate', 'callout callout-ai', 'side-lab', 'done-when']) {
    assert.match(html, new RegExp(`class="${marker}[ "]`), `missing ${marker}`);
  }
  assert.doesNotMatch(html, /not written yet|coming soon/i);
});

test('week 7 has four to six done items with kebab-case ids', () => {
  const ids = [...html.matchAll(/type="checkbox" value="([^"]+)"/g)].map((m) => m[1]);
  assert.ok(ids.length >= 4 && ids.length <= 6, `expected 4-6 done items, got ${ids.length}`);
  for (const id of ids) {
    assert.match(id, /^[a-z0-9]+(?:-[a-z0-9]+)*$/, `done id ${id} is not kebab-case`);
    assert.doesNotMatch(id, /^item-\d+$/, `done id ${id} is still a placeholder`);
  }
});
