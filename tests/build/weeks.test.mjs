import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { weeks } from '../../src/data/weeks.ts';
import { root } from './helpers.mjs';

// Checks every week page that is written, that is, every page in dist/ without stub
// text. A stub page shows "This section is not written yet" and placeholder done items.
// Each written week must have every standard box, four to six done items with real
// kebab-case ids, and at least one diagram. Week-specific checks live in weekNN.test.mjs.

const STUB = /This section is not written yet|this week is not written yet/i;

const pages = weeks.map((w) => ({
  slug: w.slug,
  html: readFileSync(join(root, 'dist/weeks', w.slug, 'index.html'), 'utf8'),
}));
const written = pages.filter((p) => !STUB.test(p.html));

test('the written-week checks cover at least the sample week', () => {
  assert.ok(
    written.some((p) => p.slug === '04-networking'),
    `week 4 is written, so it must be checked; written weeks found: ${written.map((p) => p.slug).join(', ') || 'none'}`,
  );
});

for (const { slug, html } of written) {
  test(`${slug}: every standard box is present`, () => {
    for (const marker of ['week-header', 'callout callout-observe', 'callout callout-operate', 'callout callout-ai', 'side-lab', 'done-when', 'read-with']) {
      assert.match(html, new RegExp(`class="${marker}[ "]`), `missing ${marker}`);
    }
  });

  test(`${slug}: four to six done items with kebab-case ids`, () => {
    const ids = [...html.matchAll(/type="checkbox" value="([^"]+)"/g)].map((m) => m[1]);
    assert.ok(ids.length >= 4 && ids.length <= 6, `expected 4-6 done items, got ${ids.length}`);
    assert.equal(new Set(ids).size, ids.length, 'done ids must be unique');
    for (const id of ids) {
      assert.match(id, /^[a-z0-9]+(?:-[a-z0-9]+)*$/, `done id ${id} is not kebab-case`);
      assert.doesNotMatch(id, /^item-\d+$/, `done id ${id} is still a placeholder`);
    }
  });

  test(`${slug}: at least one inline-SVG diagram`, () => {
    const figures = [...html.matchAll(/<figure class="dg"[^>]*>[\s\S]*?<\/figure>/g)].map((m) => m[0]);
    assert.ok(figures.length >= 1, 'expected at least one figure.dg');
    for (const f of figures) assert.match(f, /<svg[\s>]/, 'figure without inline <svg>');
  });
}
