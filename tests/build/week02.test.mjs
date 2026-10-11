import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { base, root } from './helpers.mjs';

// Week 2 splits the monitor into a check package and a thin main, and tests it with
// table-driven tests. These checks keep the page complete: diagrams with walkthroughs,
// the tested Go code, every standard box, working links to other weeks, and no stub text.

const html = readFileSync(join(root, 'dist/weeks/02-go-packages-testing/index.html'), 'utf8');
const figures = [...html.matchAll(/<figure class="dg"[^>]*>[\s\S]*?<\/figure>/g)].map((m) => m[0]);
// Expressive Code splits tokens into spans, so compare code on its text with tags removed.
const named = { amp: '&', lt: '<', gt: '>', quot: '"', apos: "'" };
const unescape = (s) =>
  s.replace(/&(?:#x([0-9a-f]+)|#(\d+)|(\w+));/gi, (m, hex, dec, name) =>
    hex ? String.fromCodePoint(parseInt(hex, 16)) : dec ? String.fromCodePoint(Number(dec)) : (named[name] ?? m));
const goBlocks = [...html.matchAll(/<pre data-language="go">([\s\S]*?)<\/pre>/g)].map((m) => unescape(m[1].replace(/<div class="ec-line[^"]*">/g, '\n').replace(/<[^>]+>/g, '')));
const goCode = goBlocks.join('\n');
const example = (file) => readFileSync(join(root, 'examples/go/week02/your-monitor', file), 'utf8');

test('week 2 has at least two inline-SVG diagrams, each with walkthrough steps', () => {
  assert.ok(figures.length >= 2, `expected >= 2 figure.dg, got ${figures.length}`);
  for (const f of figures) assert.match(f, /<svg[\s>]/, 'figure without inline <svg>');
  const walkthroughs = figures.filter((f) => /<ol class="dg-steps"/.test(f));
  assert.ok(walkthroughs.length >= 2, `expected >= 2 diagrams with steps, got ${walkthroughs.length}`);
});

test('week 2 shows the check package: Getter, URL with a renamed parameter, Result and ParseList', () => {
  assert.match(goCode, /type Getter interface \{\s*Get\(url string\) \(\*http\.Response, error\)\s*\}/);
  assert.match(goCode, /func URL\(client Getter, target string\) Result/);
  assert.match(goCode, /func \(r Result\) Up\(\) bool/);
  assert.match(goCode, /func \(r Result\) String\(\) string/);
  assert.match(goCode, /func ParseList\(r io\.Reader\) \(\[\]string, error\)/);
  assert.match(goCode, /defer resp\.Body\.Close\(\)/);
});

test('week 2 shows table tests with t.Run, a t.Helper helper, httptest and a fake', () => {
  assert.match(goCode, /for _, tt := range tests \{\s*t\.Run\(tt\.name, func\(t \*testing\.T\) \{/);
  assert.match(goCode, /t\.Helper\(\)/);
  assert.match(goCode, /t\.Cleanup\(srv\.Close\)/);
  assert.match(goCode, /httptest\.NewServer/);
  assert.match(goCode, /func \(f fakeGetter\) Get\(url string\) \(\*http\.Response, error\)/);
});

test('week 2 shows main.go imports with the reader module path, matching the tested example', () => {
  const src = example('main.go');
  const region = /\/\/ region: imports\n([\s\S]*?)\n\/\/ endregion: imports/.exec(src)[1].trim();
  const expected = region.replaceAll('fsp/examples/week02/your-monitor', 'github.com/<user>/your-monitor');
  assert.notEqual(expected, region, 'main.go no longer imports the check package');
  // The page renders tabs as spaces and adds line breaks, so compare with whitespace collapsed.
  const flat = (code) => code.replace(/\s+/g, ' ').trim();
  assert.ok(goBlocks.some((b) => flat(b) === flat(expected)), `no Go block on the page equals the imports of main.go:\n${expected}`);
  assert.match(goCode, /flag\.String\("file"/);
  assert.match(goCode, /check\.ParseList\(f\)/);
});

test('week 2 links to weeks 1, 3, 4 and 5 with the site base', () => {
  const hrefs = new Set([...html.matchAll(/href="([^"]+)"/g)].map((m) => m[1]));
  for (const slug of ['01-go-basics', '03-linux-processes', '04-networking', '05-concurrency']) {
    const href = `${base}weeks/${slug}/`;
    assert.ok(hrefs.has(href), `no link to ${href}`);
    assert.ok(existsSync(join(root, 'dist/weeks', slug, 'index.html')), `${href} has no page`);
  }
});

test('week 2 has every standard box and no stub text', () => {
  for (const marker of ['callout callout-observe', 'callout callout-operate', 'callout callout-ai', 'side-lab', 'done-when']) {
    assert.match(html, new RegExp(`class="${marker}[ "]`), `missing ${marker}`);
  }
  assert.doesNotMatch(html, /not written yet|coming soon/i);
});

test('week 2 has four to six done items with kebab-case ids', () => {
  const ids = [...html.matchAll(/type="checkbox" value="([^"]+)"/g)].map((m) => m[1]);
  assert.ok(ids.length >= 4 && ids.length <= 6, `expected 4-6 done items, got ${ids.length}`);
  for (const id of ids) {
    assert.match(id, /^[a-z0-9]+(?:-[a-z0-9]+)*$/, `done id ${id} is not kebab-case`);
    assert.doesNotMatch(id, /^item-\d+$/, `done id ${id} is still a placeholder`);
  }
});
