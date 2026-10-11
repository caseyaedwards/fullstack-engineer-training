import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { base, root } from './helpers.mjs';

// Week 3 turns the monitor into a long-running process that handles SIGTERM, logs to
// stdout and runs under systemd. These checks keep the page complete: diagrams with
// walkthroughs, the tested Go code and unit file, every standard box, and no stub text.

const html = readFileSync(join(root, 'dist/weeks/03-linux-processes/index.html'), 'utf8');
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
const goBlocks = blocks('go');
const goCode = goBlocks.join('\n');
const example = (file) => readFileSync(join(root, 'examples/go/week03/your-monitor', file), 'utf8');

test('week 3 has at least two inline-SVG diagrams, two of them with walkthrough steps', () => {
  assert.ok(figures.length >= 2, `expected >= 2 figure.dg, got ${figures.length}`);
  for (const f of figures) assert.match(f, /<svg[\s>]/, 'figure without inline <svg>');
  const walkthroughs = figures.filter((f) => /<ol class="dg-steps"/.test(f));
  assert.ok(walkthroughs.length >= 2, `expected >= 2 diagrams with steps, got ${walkthroughs.length}`);
});

test('week 3 shows the signal handling, the ticker loop and logging to stdout', () => {
  assert.match(goCode, /signal\.NotifyContext\(context\.Background\(\), syscall\.SIGINT, syscall\.SIGTERM\)/);
  assert.match(goCode, /log\.SetOutput\(os\.Stdout\)/);
  assert.match(goCode, /time\.NewTicker\(interval\)/);
  assert.match(goCode, /case <-ctx\.Done\(\):/);
  assert.match(goCode, /func run\(ctx context\.Context, targets \[\]string, interval time\.Duration, checkOne func\(string\) check\.Result\)/);
  assert.match(goCode, /flag\.Duration\("interval"/);
  assert.match(goCode, /cmd\.Process\.Signal\(tt\.sig\)/);
});

test('week 3 shows main.go imports with the reader module path, matching the tested example', () => {
  const src = example('main.go');
  const region = /\/\/ region: imports\n([\s\S]*?)\n\/\/ endregion: imports/.exec(src)[1].trim();
  const expected = region.replaceAll('fsp/examples/week02/your-monitor', 'github.com/your-user/your-monitor');
  const flat = (code) => code.replace(/\s+/g, ' ').trim();
  // The CodeFrom block shows the example path; the page tells the reader to swap it.
  assert.ok(goBlocks.some((b) => flat(b) === flat(region)), 'no Go block on the page equals the imports of main.go');
  assert.ok(html.includes('github.com/your-user/your-monitor/check'), `the page never names the reader import path:\n${expected}`);
});

test('week 3 shows the unit file, and its ExecStart uses only flags that main.go defines', () => {
  const unit = blocks('ini').join('\n');
  assert.match(unit, /^ExecStart=\/usr\/local\/bin\/your-monitor /m);
  assert.match(unit, /^User=your-monitor$/m);
  assert.match(unit, /^Restart=on-failure$/m);
  const execStart = /^ExecStart=(.*)$/m.exec(unit)[1];
  const main = example('main.go');
  for (const [, name] of execStart.matchAll(/\s-(\w+)/g)) {
    assert.match(main, new RegExp(`flag\\.\\w+\\("${name}"`), `ExecStart passes -${name}, which main.go does not define`);
  }
});

test('week 3 links to weeks 2, 4 and 5 with the site base', () => {
  const hrefs = new Set([...html.matchAll(/href="([^"]+)"/g)].map((m) => m[1]));
  for (const slug of ['02-go-packages-testing', '04-networking', '05-concurrency']) {
    const href = `${base}weeks/${slug}/`;
    assert.ok(hrefs.has(href), `no link to ${href}`);
    assert.ok(existsSync(join(root, 'dist/weeks', slug, 'index.html')), `${href} has no page`);
  }
});

test('week 3 has every standard box and no stub text', () => {
  for (const marker of ['callout callout-observe', 'callout callout-operate', 'callout callout-ai', 'side-lab', 'done-when']) {
    assert.match(html, new RegExp(`class="${marker}[ "]`), `missing ${marker}`);
  }
  assert.doesNotMatch(html, /not written yet|coming soon/i);
});

test('week 3 has four to six done items with kebab-case ids', () => {
  const ids = [...html.matchAll(/type="checkbox" value="([^"]+)"/g)].map((m) => m[1]);
  assert.ok(ids.length >= 4 && ids.length <= 6, `expected 4-6 done items, got ${ids.length}`);
  for (const id of ids) {
    assert.match(id, /^[a-z0-9]+(?:-[a-z0-9]+)*$/, `done id ${id} is not kebab-case`);
    assert.doesNotMatch(id, /^item-\d+$/, `done id ${id} is still a placeholder`);
  }
});
