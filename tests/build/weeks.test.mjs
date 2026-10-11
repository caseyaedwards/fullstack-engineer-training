import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { weeks } from '../../src/data/weeks.ts';
import { root, weeksDir } from './helpers.mjs';

// Checks every written week page. A week counts as written once its done items have
// real ids: stubs use the placeholders item-1, item-2 and item-3. A written week must
// carry no stub text, every standard box, four to six done items with kebab-case ids,
// and at least two diagrams, one of them with walkthrough steps.
// Week-specific checks live in weekNN.test.mjs.

const STUB = /This section is not written yet|this week is not written yet/i;
const PLACEHOLDER_ID = /^item-\d+$/;

/** Done ids straight from the source frontmatter, independent of the rendered page. */
const doneIds = (slug) =>
  [...readFileSync(join(weeksDir, `${slug}.mdx`), 'utf8').matchAll(/^\s+- id:\s*(\S+)\s*$/gm)].map((m) => m[1]);

const pages = weeks.map((w) => ({
  slug: w.slug,
  ids: doneIds(w.slug),
  html: readFileSync(join(root, 'dist/weeks', w.slug, 'index.html'), 'utf8'),
}));
const written = pages.filter((p) => !p.ids.some((id) => PLACEHOLDER_ID.test(id)));

test('every week page has done ids in its frontmatter', () => {
  for (const p of pages) assert.ok(p.ids.length > 0, `${p.slug}: no done ids found`);
});

test('the written-week checks cover the weeks known to be written', () => {
  const slugs = written.map((p) => p.slug);
  for (const slug of ['00-setup', '01-go-basics', '04-networking']) {
    assert.ok(slugs.includes(slug), `${slug} is written, so it must be checked; written weeks found: ${slugs.join(', ') || 'none'}`);
  }
});

for (const { slug, ids, html } of written) {
  test(`${slug}: no stub text`, () => {
    assert.doesNotMatch(html, STUB);
  });

  test(`${slug}: every standard box is present`, () => {
    for (const marker of ['week-header', 'callout callout-observe', 'callout callout-operate', 'callout callout-ai', 'side-lab', 'done-when', 'read-with']) {
      assert.match(html, new RegExp(`class="${marker}[ "]`), `missing ${marker}`);
    }
  });

  test(`${slug}: four to six done items with kebab-case ids`, () => {
    const rendered = [...html.matchAll(/type="checkbox" value="([^"]+)"/g)].map((m) => m[1]);
    assert.deepEqual(rendered, ids, 'rendered done items differ from the frontmatter');
    assert.ok(ids.length >= 4 && ids.length <= 6, `expected 4-6 done items, got ${ids.length}`);
    assert.equal(new Set(ids).size, ids.length, 'done ids must be unique');
    for (const id of ids) assert.match(id, /^[a-z0-9]+(?:-[a-z0-9]+)*$/, `done id ${id} is not kebab-case`);
  });

  test(`${slug}: at least two inline-SVG diagrams, one with walkthrough steps`, () => {
    const figures = [...html.matchAll(/<figure class="dg"[^>]*>[\s\S]*?<\/figure>/g)].map((m) => m[0]);
    assert.ok(figures.length >= 2, `expected at least two figure.dg, got ${figures.length}`);
    for (const f of figures) assert.match(f, /<svg[\s>]/, 'figure without inline <svg>');
    assert.ok(figures.some((f) => /<ol class="dg-steps"/.test(f)), 'no diagram with walkthrough steps');
  });
}
