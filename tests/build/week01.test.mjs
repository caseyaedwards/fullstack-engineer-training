import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { base, root } from './helpers.mjs';

// Week 1 teaches Go basics through the first version of the monitor. These checks keep
// the page complete: diagrams with walkthroughs, the tested Go code, every standard box,
// working links to other weeks, and no stub text.

const html = readFileSync(join(root, 'dist/weeks/01-go-basics/index.html'), 'utf8');
const figures = [...html.matchAll(/<figure class="dg"[^>]*>[\s\S]*?<\/figure>/g)].map((m) => m[0]);
// Expressive Code splits tokens into spans, so compare code on its text with tags removed.
const named = { amp: '&', lt: '<', gt: '>', quot: '"', apos: "'" };
const unescape = (s) =>
  s.replace(/&(?:#x([0-9a-f]+)|#(\d+)|(\w+));/gi, (m, hex, dec, name) =>
    hex ? String.fromCodePoint(parseInt(hex, 16)) : dec ? String.fromCodePoint(Number(dec)) : (named[name] ?? m));
const goBlocks = [...html.matchAll(/<pre data-language="go">([\s\S]*?)<\/pre>/g)].map((m) => unescape(m[1].replace(/<div class="ec-line[^"]*">/g, '\n').replace(/<[^>]+>/g, '')));
const goCode = goBlocks.join('\n');

test('week 1 has at least three inline-SVG diagrams, two of them with walkthrough steps', () => {
  assert.ok(figures.length >= 3, `expected >= 3 figure.dg, got ${figures.length}`);
  for (const f of figures) assert.match(f, /<svg[\s>]/, 'figure without inline <svg>');
  const walkthroughs = figures.filter((f) => /<ol class="dg-steps"/.test(f));
  assert.ok(walkthroughs.length >= 2, `expected >= 2 diagrams with steps, got ${walkthroughs.length}`);
});

test('week 1 shows checkURL with the signature Week 2 builds on', () => {
  assert.match(goCode, /func checkURL\(client \*http\.Client, url string\) \(status int, latency time\.Duration, err error\)/);
  assert.match(goCode, /start := time\.Now\(\)/);
  assert.match(goCode, /time\.Since\(start\)/);
  assert.match(goCode, /defer resp\.Body\.Close\(\)/);
});

test('week 1 shows a main that reads -url, sets a client timeout and exits non-zero on failure', () => {
  assert.match(goCode, /flag\.String\("url"/);
  assert.match(goCode, /&http\.Client\{Timeout: \*timeout\}/);
  assert.match(goCode, /os\.Exit\(1\)/);
  assert.match(goCode, /os\.Exit\(2\)/);
});

test('week 1 links to weeks 0, 2, 3 and 4 with the site base', () => {
  const hrefs = new Set([...html.matchAll(/href="([^"]+)"/g)].map((m) => m[1]));
  for (const slug of ['00-setup', '02-go-packages-testing', '03-linux-processes', '04-networking']) {
    const href = `${base}weeks/${slug}/`;
    assert.ok(hrefs.has(href), `no link to ${href}`);
    assert.ok(existsSync(join(root, 'dist/weeks', slug, 'index.html')), `${href} has no page`);
  }
});

test('week 1 has every standard box and no stub text', () => {
  for (const marker of ['callout callout-observe', 'callout callout-operate', 'callout callout-ai', 'side-lab', 'done-when']) {
    assert.match(html, new RegExp(`class="${marker}[ "]`), `missing ${marker}`);
  }
  assert.doesNotMatch(html, /not written yet|coming soon/i);
});

test('week 1 has four to six done items with kebab-case ids', () => {
  const ids = [...html.matchAll(/type="checkbox" value="([^"]+)"/g)].map((m) => m[1]);
  assert.ok(ids.length >= 4 && ids.length <= 6, `expected 4-6 done items, got ${ids.length}`);
  for (const id of ids) {
    assert.match(id, /^[a-z0-9]+(?:-[a-z0-9]+)*$/, `done id ${id} is not kebab-case`);
    assert.doesNotMatch(id, /^item-\d+$/, `done id ${id} is still a placeholder`);
  }
});
