import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { root } from './helpers.mjs';

// Week 14 checks: diagrams with walkthroughs, every standard box, no stub text, the
// done items, and the workflow code on the page: every action pinned to a full
// commit SHA with a version comment, and every job of ci.yml shown.

const html = readFileSync(join(root, 'dist/weeks/14-ci-cd/index.html'), 'utf8');
const figures = [...html.matchAll(/<figure class="dg"[^>]*>[\s\S]*?<\/figure>/g)].map((m) => m[0]);
// Expressive Code splits tokens into spans, so compare code on its text with tags removed.
const decode = (s) => s.replace(/<[^>]+>/g, '').replace(/&#x27;|&#39;/g, "'").replace(/&quot;/g, '"').replace(/&gt;/g, '>').replace(/&lt;/g, '<').replace(/&amp;/g, '&');
const blocks = (lang) =>
  [...html.matchAll(new RegExp(`<pre data-language="${lang}">([\\s\\S]*?)<\\/pre>`, 'g'))].map((m) =>
    decode(m[1].replace(/<\/div>/g, '\n')),
  );
const yaml = blocks('yaml').join('\n');

test('week 14 has at least two inline-SVG diagrams with walkthrough steps', () => {
  assert.ok(figures.length >= 2, `expected >= 2 figure.dg, got ${figures.length}`);
  for (const f of figures) assert.match(f, /<svg[\s>]/, 'figure without inline <svg>');
  const walkthroughs = figures.filter((f) => /<ol class="dg-steps"/.test(f));
  assert.ok(walkthroughs.length >= 2, `expected >= 2 diagrams with steps, got ${walkthroughs.length}`);
});

test('week 14 has every standard box and no stub text', () => {
  for (const marker of ['callout callout-observe', 'callout callout-operate', 'callout callout-ai', 'side-lab', 'done-when']) {
    assert.match(html, new RegExp(`class="${marker}[ "]`), `missing ${marker}`);
  }
  assert.doesNotMatch(html, /not written yet|coming soon/i);
});

test('week 14 has four to six done items with kebab-case ids', () => {
  const ids = [...html.matchAll(/type="checkbox" value="([^"]+)"/g)].map((m) => m[1]);
  assert.ok(ids.length >= 4 && ids.length <= 6, `expected 4-6 done items, got ${ids.length}`);
  for (const id of ids) {
    assert.match(id, /^[a-z0-9]+(?:-[a-z0-9]+)*$/, `done id ${id} is not kebab-case`);
    assert.doesNotMatch(id, /^item-\d+$/, `done id ${id} is still a placeholder`);
  }
});

test('week 14 pins every action on the page to a full commit SHA with a version comment', () => {
  const uses = [...yaml.matchAll(/uses: (\S+)(.*)/g)];
  assert.ok(uses.length >= 10, `expected the workflows on the page, found ${uses.length} uses: lines`);
  for (const [line, ref, rest] of uses) {
    assert.match(ref, /^[\w.-]+\/[\w.-]+@[0-9a-f]{40}$/, `not pinned to a full SHA: ${line}`);
    assert.match(rest, /^ # v\d+\.\d+\.\d+$/, `no version comment: ${line}`);
  }
});

test('week 14 shows every job of ci.yml, the Postgres service and the manual deploy', () => {
  for (const job of ['lint', 'test', 'web', 'e2e', 'images']) {
    assert.match(yaml, new RegExp(`^  ${job}:$`, 'm'), `ci.yml job ${job} is not on the page`);
  }
  assert.match(yaml, /image: postgres:17/);
  assert.match(yaml, /packages: write/);
  assert.match(yaml, /workflow_dispatch:/);
  assert.match(yaml, /SHA: \$\{\{ inputs\.sha \|\| github\.sha \}\}/);
});
