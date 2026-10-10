import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { weeks } from '../../src/data/weeks.ts';
import { base, root } from './helpers.mjs';

const dist = (p) => readFileSync(join(root, 'dist', p, 'index.html'), 'utf8');
const home = dist('');

test('every week page has a Done when checklist whose data-total matches its checkboxes', () => {
  for (const w of weeks) {
    const html = dist(`weeks/${w.slug}`);
    const block = html.match(/<div class="done-when[^>]*>[\s\S]*?<\/ul>/)?.[0];
    assert.ok(block, `${w.slug}: no .done-when block`);
    const total = Number(block.match(/data-total="(\d+)"/)?.[1]);
    assert.equal(Number(block.match(/data-week="(\d+)"/)?.[1]), w.n, `${w.slug}: data-week`);
    assert.equal((block.match(/type="checkbox"/g) ?? []).length, total, `${w.slug}: checkbox count vs data-total`);
    assert.ok(total >= 1, `${w.slug}: empty checklist`);
    assert.match(html, /<h2 id="done-when"/, `${w.slug}: the checklist is labelled by #done-when, which has no heading`);
  }
});

test('the home timeline has one node per week, with the same total as that week page and a base-aware link', () => {
  for (const w of weeks) {
    const li = home.match(new RegExp(`<li[^>]*data-week="${w.n}"[^>]*>[\\s\\S]*?</li>`))?.[0];
    assert.ok(li, `week ${w.n}: not on the timeline`);
    const timelineTotal = Number(li.match(/data-total="(\d+)"/)?.[1]);
    const pageTotal = Number(dist(`weeks/${w.slug}`).match(/class="done-when[^>]*data-total="(\d+)"/)?.[1]);
    assert.equal(timelineTotal, pageTotal, `week ${w.n}: timeline total drifted from the page`);
    const ids = JSON.parse(li.match(/data-ids="([^"]*)"/)[1].replaceAll('&#34;', '"').replaceAll('&quot;', '"'));
    const boxes = [...dist(`weeks/${w.slug}`).matchAll(/type="checkbox" value="([^"]+)"/g)].map((m) => m[1]);
    assert.deepEqual(ids, boxes, `week ${w.n}: timeline ids differ from the page's checkboxes`);
    assert.ok(li.includes(`href="${base}weeks/${w.slug}/"`), `week ${w.n}: link is not ${base}weeks/${w.slug}/`);
  }
});

test('the home page has the start link under the site base', () => {
  assert.ok(home.includes(`href="${base}weeks/00-setup/"`));
  assert.match(home, /Start at week 0/);
});

test('callout asides are labelled by a heading that exists on the page', () => {
  for (const w of weeks) {
    const html = dist(`weeks/${w.slug}`);
    // Only the site's own callouts; Starlight's :::note asides carry their own aria-label.
    const callouts = [...html.matchAll(/<aside class="callout [^>]*>/g)].map((m) => m[0]);
    const refs = callouts.map((a) => a.match(/aria-labelledby="([^"]+)"/)?.[1]);
    assert.equal(refs.length, 3, `${w.slug}: expected 3 callouts`);
    for (const id of refs) assert.match(html, new RegExp(`<h2 id="${id}"`), `${w.slug}: no heading #${id}`);
    assert.ok(!callouts.some((a) => /aria-label="/.test(a)), `${w.slug}: a callout still has a duplicate aria-label`);
  }
});

test('each timeline phase section is labelled by a heading on the home page', () => {
  const refs = [...home.matchAll(/<section[^>]*aria-labelledby="([^"]+)"/g)].map((m) => m[1]);
  assert.equal(refs.length, 4);
  for (const id of refs) assert.match(home, new RegExp(`<h3 id="${id}"`));
});
