import { test } from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { base, root } from './helpers.mjs';

// Week 10 builds the status page in web/ and serves it from nginx in Compose. These
// checks keep the page complete, keep the snippets on the page in step with the tested
// example files, and check that the web image, nginx, Compose and the mocked api agree.

const html = readFileSync(join(root, 'dist/weeks/10-frontend/index.html'), 'utf8');
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
const goDir = join(root, 'examples/go/week10/your-monitor');
const webDir = join(root, 'examples/web/src/week10/web');
const read = (dir, file) => readFileSync(join(dir, file), 'utf8');

test('week 10 has four inline-SVG diagrams, three of them with walkthrough steps', () => {
  assert.ok(figures.length >= 4, `expected >= 4 figure.dg, got ${figures.length}`);
  for (const f of figures) assert.match(f, /<svg[\s>]/, 'figure without inline <svg>');
  for (const id of ['page-data', 'polling', 'test-levels']) {
    assert.ok(figures.some((f) => f.includes(`id="${id}"`) && f.includes('dg-steps')), `diagram ${id} has no walkthrough`);
  }
});

test('week 10 shows the hook with its cleanup, the typed client and the chart math', () => {
  const ts = [...blocks('ts'), ...blocks('tsx')].join('\n');
  assert.match(ts, /const controller = new AbortController\(\)/);
  assert.match(ts, /controller\.abort\(\)\s*\n\s*clearTimeout\(timer\)/);
  assert.match(ts, /\}, \[load, everyMs\]\)/);
  assert.match(ts, /export class ApiError extends Error/);
  assert.match(ts, /`\/api\/monitors\/\$\{monitorID\}\/results\?limit=\$\{limit\}`/);
  assert.match(ts, /'\/api\/incidents\?open=true'/);
  assert.match(ts, /export function plot\(results: Result\[\], width: number, height: number\): Plot/);
  assert.match(ts, /expect\(p\.path\)\.toBe\('M0,30 L100,0 M300,45'\)/);
  assert.match(ts, /role="img"/);
});

test('week 10 shows the Testing Library tests and the Playwright flow with a mocked api', () => {
  const ts = [...blocks('ts'), ...blocks('tsx')].join('\n');
  assert.match(ts, /environment: 'jsdom'/);
  assert.match(ts, /include: \['src\/\*\*\/\*\.test\.\{ts,tsx\}'\]/);
  assert.match(ts, /screen\.getByRole\('listitem', \{ name: url \}\)/);
  assert.match(ts, /vi\.advanceTimersByTimeAsync\(30_000\)/);
  assert.match(ts, /await page\.route\('\*\*\/api\/\*\*'/);
  assert.match(ts, /await page\.clock\.install\(/);
  assert.match(ts, /await page\.clock\.runFor\(30_000\)/);
});

test('week 10 shows the CSS region without its comment markers', () => {
  const css = blocks('css').join('\n');
  assert.match(css, /\.badge\.down \{ background: var\(--down\); \}/);
  assert.doesNotMatch(css, /region: css/, 'CSS region markers must not be rendered');
});

test('the .dockerignore on the page matches the example file and lets web/ into the build', () => {
  const want = read(goDir, '.dockerignore').trimEnd();
  assert.ok(blocks('txt').some((b) => b.trimEnd() === want), `no txt block on the page equals .dockerignore:\n${want}`);
  const lines = want.split('\n');
  assert.ok(!lines.includes('web'), '.dockerignore must not exclude all of web/');
  for (const line of ['web/node_modules', 'web/dist', '.env']) assert.ok(lines.includes(line), `.dockerignore must list ${line}`);
});

test('nginx, the Dockerfile web target, Compose and Vite agree on ports and paths', () => {
  const nginx = read(webDir, 'nginx.conf');
  const docker = read(goDir, 'Dockerfile');
  const compose = read(goDir, 'compose.yaml');
  const listen = /^\s*listen (\d+);/m.exec(nginx)?.[1];
  assert.ok(listen, 'nginx.conf has no listen line');
  assert.match(nginx, /proxy_pass http:\/\/api:8080;/);
  assert.match(read(goDir, 'cmd/api/main.go'), /flag\.String\("addr", ":8080"/, 'the api must listen on 8080, where nginx sends /api/');
  assert.match(docker, /FROM nginxinc\/nginx-unprivileged:[\w.-]+ AS web\n/);
  assert.match(docker, /COPY web\/nginx\.conf \/etc\/nginx\/conf\.d\/default\.conf\n/);
  assert.match(docker, /COPY --from=web-build \/web\/dist \/usr\/share\/nginx\/html\n/);
  assert.match(nginx, /root \/usr\/share\/nginx\/html;/);
  assert.match(docker, new RegExp(`EXPOSE ${listen}\\n# endregion: web`));
  assert.match(compose, new RegExp(`"127\\.0\\.0\\.1:\\$\\{WEB_PORT:-3000\\}:${listen}"`));
  assert.match(read(webDir, 'vite.config.ts'), /'\/api': 'http:\/\/127\.0\.0\.1:8080'/);
});

test('the mocked api in the Playwright test sends every field of the Result type', () => {
  const api = read(webDir, 'src/api.ts');
  const type = /export type Result = \{([\s\S]*?)\n\}/.exec(api)?.[1];
  assert.ok(type, 'type Result not found in api.ts');
  const fields = [...type.matchAll(/^\s+(\w+):/gm)].map((m) => m[1]).sort();
  const spec = read(webDir, 'e2e/status.spec.ts');
  for (const builder of ['up', 'down']) {
    const body = new RegExp(`const ${builder} = \\([^)]*\\) => \\(\\{([\\s\\S]*?)\\}\\)`).exec(spec)?.[1];
    assert.ok(body, `builder ${builder} not found in status.spec.ts`);
    const keys = [...body.matchAll(/(\w+):/g)].map((m) => m[1]).sort();
    assert.deepEqual(keys, fields, `${builder}() in status.spec.ts must send exactly the fields of Result`);
  }
});

// docker compose config parses and checks the file without a Docker engine. CI has the
// Docker CLI with Compose; a machine without it skips this test, except in CI.
const haveCompose = spawnSync('docker', ['compose', 'version'], { encoding: 'utf8' }).status === 0;
test('docker compose config gives the web service the expected shape', { skip: !haveCompose && !process.env.CI && 'docker compose is not installed' }, () => {
  assert.ok(haveCompose, 'docker compose must be installed in CI');
  const out = execFileSync('docker', ['compose', '--env-file', '.env.example', 'config', '--format', 'json'], { cwd: goDir, encoding: 'utf8' });
  const { services } = JSON.parse(out);
  const web = services.web;
  assert.ok(web, 'compose.yaml has no web service');
  assert.equal(web.build.target, 'web');
  assert.equal(web.depends_on?.api?.condition, 'service_healthy', 'web must wait for a healthy api');
  assert.deepEqual(
    web.ports.map((p) => [p.host_ip, String(p.published), p.target]),
    [['127.0.0.1', '3000', 8080]],
  );
});

test('week 10 links to weeks 6, 7, 9 and 15 with the site base', () => {
  const hrefs = new Set([...html.matchAll(/href="([^"]+)"/g)].map((m) => m[1]));
  for (const slug of ['06-architecture-repo', '07-backend-api', '09-jobs-alerts', '15-kubernetes-local']) {
    const href = `${base}weeks/${slug}/`;
    assert.ok(hrefs.has(href), `no link to ${href}`);
    assert.ok(existsSync(join(root, 'dist/weeks', slug, 'index.html')), `${href} has no page`);
  }
});

test('week 10 has every standard box and no stub text', () => {
  for (const marker of ['callout callout-observe', 'callout callout-operate', 'callout callout-ai', 'side-lab', 'done-when']) {
    assert.match(html, new RegExp(`class="${marker}[ "]`), `missing ${marker}`);
  }
  assert.doesNotMatch(html, /not written yet|coming soon/i);
});

test('week 10 has four to six done items with kebab-case ids', () => {
  const ids = [...html.matchAll(/type="checkbox" value="([^"]+)"/g)].map((m) => m[1]);
  assert.ok(ids.length >= 4 && ids.length <= 6, `expected 4-6 done items, got ${ids.length}`);
  for (const id of ids) {
    assert.match(id, /^[a-z0-9]+(?:-[a-z0-9]+)*$/, `done id ${id} is not kebab-case`);
    assert.doesNotMatch(id, /^item-\d+$/, `done id ${id} is still a placeholder`);
  }
});
