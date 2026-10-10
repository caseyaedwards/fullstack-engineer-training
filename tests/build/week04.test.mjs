import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { root } from './helpers.mjs';

// Week 4 is the fully written sample week. These checks keep it complete:
// diagrams with walkthroughs, the tested Go code, every standard box, and no stub text.

const html = readFileSync(join(root, 'dist/weeks/04-networking/index.html'), 'utf8');
const figures = [...html.matchAll(/<figure class="dg"[^>]*>[\s\S]*?<\/figure>/g)].map((m) => m[0]);
// Expressive Code splits tokens into spans, so compare code on its text with tags removed.
const goBlocks = [...html.matchAll(/<pre data-language="go">([\s\S]*?)<\/pre>/g)].map((m) => m[1].replace(/<[^>]+>/g, ''));

test('week 4 has at least three inline-SVG diagrams, two of them with walkthrough steps', () => {
  assert.ok(figures.length >= 3, `expected >= 3 figure.dg, got ${figures.length}`);
  for (const f of figures) assert.match(f, /<svg[\s>]/, 'figure without inline <svg>');
  const walkthroughs = figures.filter((f) => /<ol class="dg-steps"/.test(f));
  assert.ok(walkthroughs.length >= 2, `expected >= 2 diagrams with steps, got ${walkthroughs.length}`);
});

test('week 4 shows at least two Go code blocks that use httptrace', () => {
  const traced = goBlocks.filter((b) => b.includes('httptrace'));
  assert.ok(traced.length >= 2, `expected >= 2 Go blocks with httptrace, got ${traced.length}`);
});

test('week 4 shows the helpers that the timings snippet calls', () => {
  const code = goBlocks.join('\n');
  assert.match(code, /httptrace\.ClientTrace/);
  for (const helper of ['begin', 'end']) {
    assert.match(code, new RegExp(`\\b${helper} := func`), `${helper} is used on the page but never shown`);
  }
});

test('week 4 has every standard box and no stub text', () => {
  for (const marker of ['callout callout-observe', 'callout callout-operate', 'callout callout-ai', 'side-lab', 'done-when']) {
    assert.match(html, new RegExp(`class="${marker}[ "]`), `missing ${marker}`);
  }
  assert.doesNotMatch(html, /coming soon/i);
});

test('week 4 has four to six done items with kebab-case ids', () => {
  const ids = [...html.matchAll(/type="checkbox" value="([^"]+)"/g)].map((m) => m[1]);
  assert.ok(ids.length >= 4 && ids.length <= 6, `expected 4-6 done items, got ${ids.length}`);
  for (const id of ids) {
    assert.match(id, /^[a-z0-9]+(?:-[a-z0-9]+)*$/, `done id ${id} is not kebab-case`);
    assert.doesNotMatch(id, /^item-\d+$/, `done id ${id} is still a placeholder`);
  }
});
