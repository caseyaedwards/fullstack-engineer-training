import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { base, root } from './helpers.mjs';

// Week 9 splits the monitor into a scheduler, a job queue in Postgres, workers
// and an alerter. These checks keep the page complete: diagrams with
// walkthroughs, the claim query and its transaction, the concurrent test, the
// migration, every standard box, and drill output on the page that matches
// the log formats the example code prints.

const html = readFileSync(join(root, 'dist/weeks/09-jobs-alerts/index.html'), 'utf8');
const figures = [...html.matchAll(/<figure class="dg"[^>]*>[\s\S]*?<\/figure>/g)].map((m) => m[0]);
// Expressive Code splits tokens into spans, so compare code on its text with tags removed.
const named = { amp: '&', lt: '<', gt: '>', quot: '"', apos: "'" };
const unescape = (s) =>
  s.replace(/&(?:#x([0-9a-f]+)|#(\d+)|(\w+));/gi, (m, hex, dec, name) =>
    hex ? String.fromCodePoint(parseInt(hex, 16)) : dec ? String.fromCodePoint(Number(dec)) : (named[name] ?? m));
const blocks = (lang) =>
  [...html.matchAll(new RegExp(`<pre data-language="${lang}"[^>]*>([\\s\\S]*?)</pre>`, 'g'))].map((m) =>
    unescape(m[1].replace(/<div class="ec-line[^"]*">/g, '\n').replace(/<[^>]+>/g, '')),
  );
const goCode = blocks('go').join('\n');
const sqlCode = blocks('sql').join('\n');
const txt = blocks('txt').join('\n');
const example = (file) => readFileSync(join(root, 'examples/go/week09/your-monitor', file), 'utf8');

test('week 9 has at least three inline-SVG diagrams, two of them with walkthrough steps', () => {
  assert.ok(figures.length >= 3, `expected >= 3 figure.dg, got ${figures.length}`);
  for (const f of figures) assert.match(f, /<svg[\s>]/, 'figure without inline <svg>');
  const walked = figures.filter((f) => /<ol class="dg-steps"/.test(f));
  assert.ok(walked.length >= 2, `expected >= 2 walkthroughs, got ${walked.length}`);
});

test('week 9 shows the claim query, its transaction and the enqueue statement', () => {
  assert.match(goCode, /FOR UPDATE OF j SKIP LOCKED/);
  assert.match(goCode, /tx, err := db\.Begin\(ctx\)/);
  assert.match(goCode, /defer tx\.Rollback\(ctx\)/);
  assert.match(goCode, /return true, tx\.Commit\(ctx\)/);
  assert.match(goCode, /DELETE FROM check_jobs WHERE id = \$1/);
  assert.match(goCode, /SET last_enqueued_at = now\(\)/);
  assert.match(goCode, /NOT EXISTS \(SELECT 1 FROM check_jobs j WHERE j\.monitor_id = m\.id\)/);
});

test('week 9 shows the concurrent test that proves no job runs twice', () => {
  assert.match(goCode, /func TestEachJobRunsOnce\(t \*testing\.T\)/);
  assert.match(goCode, /for range 4 \{/);
  assert.match(goCode, /t\.Errorf\("job %d ran %d times, want 1", id, n\)/);
  assert.match(goCode, /maxPar\.Load\(\); got < 2/);
});

test('week 9 shows the migration, with the queue table and the one-open-incident index', () => {
  assert.match(sqlCode, /CREATE TABLE check_jobs \(/);
  assert.doesNotMatch(sqlCode, /region: /, 'SQL region markers must not be rendered');
  assert.match(sqlCode, /ALTER TABLE monitors ADD COLUMN last_enqueued_at timestamptz;/);
  assert.match(sqlCode, /CREATE UNIQUE INDEX incidents_one_open ON incidents \(monitor_id\) WHERE resolved_at IS NULL;/);
});

test('week 9 drill output uses the log formats of the worker and the log notifier', () => {
  // logCheck in cmd/worker/main.go and Log.Notify in internal/alert/notify.go.
  const checkFormat = /line := fmt\.Sprintf\("(check [^"]+)"/.exec(example('cmd/worker/main.go'));
  const alertFormat = /l\.Logger\.Printf\("(alert [^"]+)"/.exec(example('internal/alert/notify.go'));
  assert.ok(checkFormat && alertFormat, 'log formats not found in the example code');
  const keys = (format) => [...format.matchAll(/(\w+)=%/g)].map((m) => m[1]);
  const checkKeys = keys(checkFormat[1]);
  const alertKeys = keys(alertFormat[1]);

  const checkLines = [...txt.matchAll(/^\S+ \S+ (check url=.*)$/gm)].map((m) => m[1]);
  const alertLines = [...txt.matchAll(/^\S+ \S+ alert (opened|resolved) (.*)$/gm)].map((m) => m[2]);
  assert.ok(checkLines.length >= 4, `expected >= 4 check lines, got ${checkLines.length}`);
  assert.ok(alertLines.length >= 2, `expected an opened and a resolved alert line, got ${alertLines.length}`);
  for (const line of checkLines) {
    const got = [...line.matchAll(/(\w+)=/g)].map((m) => m[1]).filter((k) => k !== 'error');
    assert.deepEqual(got, checkKeys, `check line does not match logCheck: ${line}`);
  }
  for (const line of alertLines) {
    const got = [...line.matchAll(/(\w+)=/g)].map((m) => m[1]);
    assert.deepEqual(got, alertKeys, `alert line does not match Log.Notify: ${line}`);
  }
});

test('week 9 links to weeks 8 and the messaging track with the site base', () => {
  const hrefs = new Set([...html.matchAll(/href="([^"]+)"/g)].map((m) => m[1]));
  const href = `${base}weeks/08-backend-postgres/`;
  assert.ok(hrefs.has(href), `no link to ${href}`);
  assert.ok(existsSync(join(root, 'dist/weeks/08-backend-postgres/index.html')), `${href} has no page`);
  assert.ok(hrefs.has(`${base}tracks/building/messaging/`), 'no link to the messaging track');
});

test('week 9 has every standard box and no stub text', () => {
  for (const marker of ['callout callout-observe', 'callout callout-operate', 'callout callout-ai', 'side-lab', 'done-when']) {
    assert.match(html, new RegExp(`class="${marker}[ "]`), `missing ${marker}`);
  }
  assert.doesNotMatch(html, /not written yet|coming soon/i);
});

test('week 9 has four to six done items with kebab-case ids', () => {
  const ids = [...html.matchAll(/type="checkbox" value="([^"]+)"/g)].map((m) => m[1]);
  assert.ok(ids.length >= 4 && ids.length <= 6, `expected 4-6 done items, got ${ids.length}`);
  for (const id of ids) {
    assert.match(id, /^[a-z0-9]+(?:-[a-z0-9]+)*$/, `done id ${id} is not kebab-case`);
    assert.doesNotMatch(id, /^item-\d+$/, `done id ${id} is still a placeholder`);
  }
});
