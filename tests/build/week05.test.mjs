import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { base, root } from './helpers.mjs';

// Week 5 runs each round of checks through a worker pool, with a timeout per check,
// cancellation on SIGTERM and a timeout count per round. These checks keep the page
// complete: diagrams with walkthroughs, the tested pool code, every standard box,
// and round lines on the page that match the summary the example code prints.

const html = readFileSync(join(root, 'dist/weeks/05-concurrency/index.html'), 'utf8');
const figures = [...html.matchAll(/<figure class="dg"[^>]*>[\s\S]*?<\/figure>/g)].map((m) => m[0]);
// Expressive Code splits tokens into spans, so compare code on its text with tags removed.
const named = { amp: '&', lt: '<', gt: '>', quot: '"', apos: "'" };
const unescape = (s) =>
  s.replace(/&(?:#x([0-9a-f]+)|#(\d+)|(\w+));/gi, (m, hex, dec, name) =>
    hex ? String.fromCodePoint(parseInt(hex, 16)) : dec ? String.fromCodePoint(Number(dec)) : (named[name] ?? m));
const blocks = (lang) =>
  [...html.matchAll(new RegExp(`<pre data-language="${lang}">([\\s\\S]*?)</pre>`, 'g'))].map((m) =>
    unescape(m[1].replace(/<div class="ec-line[^"]*">/g, '\n').replace(/<[^>]+>/g, '')),
  );
const goCode = blocks('go').join('\n');
const txt = blocks('txt').join('\n');
const example = (file) => readFileSync(join(root, 'examples/go/week05/your-monitor', file), 'utf8');

test('week 5 has at least three inline-SVG diagrams, each with walkthrough steps', () => {
  assert.ok(figures.length >= 3, `expected >= 3 figure.dg, got ${figures.length}`);
  for (const f of figures) {
    assert.match(f, /<svg[\s>]/, 'figure without inline <svg>');
    assert.match(f, /<ol class="dg-steps"/, 'figure without walkthrough steps');
  }
});

test('week 5 shows the whole pool: workers, feeder, closer and collector', () => {
  assert.match(goCode, /jobs := make\(chan string\)/);
  assert.match(goCode, /results := make\(chan check\.Result\)/);
  assert.match(goCode, /for target := range jobs \{/);
  assert.match(goCode, /results <- checkOne\(ctx, target\)/);
  assert.match(goCode, /case jobs <- target:/);
  assert.match(goCode, /case <-ctx\.Done\(\):/);
  assert.match(goCode, /defer close\(jobs\)/);
  assert.match(goCode, /wg\.Wait\(\)\s*\n\s*close\(results\)/);
  assert.match(goCode, /for r := range results \{/);
});

test('week 5 shows the 100-URL test, the leak check and the -workers command-line flag', () => {
  assert.match(goCode, /for i := range 100 \{/);
  assert.match(goCode, /pprof\.Lookup\("goroutine"\)\.WriteTo\(&buf, 2\)/);
  assert.match(goCode, /func checkNoLeaks\(t \*testing\.T\)/);
  assert.match(goCode, /flag\.Int\("workers"/);
  assert.match(goCode, /context\.WithTimeout\(ctx, \*timeout\)/);
  assert.match(goCode, /pool\.Run\(ctx, targets, workers, checkOne\)/);
  assert.match(goCode, /errors\.Is\(r\.Err, context\.DeadlineExceeded\)/);
});

test('week 5 round lines on the page use the fields that summary.String prints, in its order', () => {
  // The format string of summary.String, e.g. "checks=%d up=%d ...", gives the field order.
  const format = /func \(s summary\) String\(\) string \{\s*return fmt\.Sprintf\("([^"]+)"/.exec(example('main.go'));
  assert.ok(format, 'summary.String not found in main.go');
  const fields = [...format[1].matchAll(/(\w+)=%d/g)].map((m) => m[1]);
  assert.ok(fields.includes('timeouts'), 'the round summary must count timeouts');
  const rounds = [...txt.matchAll(/ round (\w+=.*)$/gm)].map((m) => m[1]);
  assert.ok(rounds.length >= 3, `expected >= 3 round lines in the page output, got ${rounds.length}`);
  for (const line of rounds) {
    const keys = [...line.matchAll(/(\w+)=\d+/g)].map((m) => m[1]);
    assert.deepEqual(keys.slice(0, fields.length), fields, `round line does not match summary.String: ${line}`);
    assert.deepEqual(keys.slice(fields.length), ['duration_ms', 'goroutines'], `round line ends differently from run: ${line}`);
    const n = Object.fromEntries([...line.matchAll(/(\w+)=(\d+)/g)].map((m) => [m[1], Number(m[2])]));
    assert.equal(n.up + n.down + n.cancelled, n.checks, `up + down + cancelled != checks: ${line}`);
    assert.ok(n.timeouts <= n.down, `more timeouts than down checks: ${line}`);
  }
});

test('week 5 links to weeks 2, 3, 4 and 6 with the site base', () => {
  const hrefs = new Set([...html.matchAll(/href="([^"]+)"/g)].map((m) => m[1]));
  for (const slug of ['02-go-packages-testing', '03-linux-processes', '04-networking', '06-architecture-repo']) {
    const href = `${base}weeks/${slug}/`;
    assert.ok(hrefs.has(href), `no link to ${href}`);
    assert.ok(existsSync(join(root, 'dist/weeks', slug, 'index.html')), `${href} has no page`);
  }
});

test('week 5 has every standard box and no stub text', () => {
  for (const marker of ['callout callout-observe', 'callout callout-operate', 'callout callout-ai', 'side-lab', 'done-when']) {
    assert.match(html, new RegExp(`class="${marker}[ "]`), `missing ${marker}`);
  }
  assert.doesNotMatch(html, /not written yet|coming soon/i);
});

test('week 5 has four to six done items with kebab-case ids', () => {
  const ids = [...html.matchAll(/type="checkbox" value="([^"]+)"/g)].map((m) => m[1]);
  assert.ok(ids.length >= 4 && ids.length <= 6, `expected 4-6 done items, got ${ids.length}`);
  for (const id of ids) {
    assert.match(id, /^[a-z0-9]+(?:-[a-z0-9]+)*$/, `done id ${id} is not kebab-case`);
    assert.doesNotMatch(id, /^item-\d+$/, `done id ${id} is still a placeholder`);
  }
});
