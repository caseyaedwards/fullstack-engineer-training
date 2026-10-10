import { test } from 'node:test';
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { weeks } from '../../src/data/weeks.ts';
import { tracks } from '../../src/data/tracks.ts';
import { base, frontmatter, normalizeBase, root, weeksDir } from './helpers.mjs';

const weekPath = (n) => `weeks/${weeks.find((w) => w.n === n).slug}/`;
const trackPath = (slug) => {
  const t = tracks.find((x) => x.slug === slug);
  return `tracks/${t.group}/${t.slug}/`;
};

const page = (dist, path) => readFileSync(join(dist, path, 'index.html'), 'utf8');
const dist = join(root, 'dist');

/** hrefs inside the first element matching `open` (e.g. '<p class="used-in-weeks ') up to its closing tag. */
function hrefsIn(html, open, close) {
  const start = html.indexOf(open);
  assert.ok(start >= 0, `${open} not found`);
  const end = html.indexOf(close, start);
  assert.ok(end > start, `${close} not found after ${open}`);
  return [...html.slice(start, end).matchAll(/href="([^"]+)"/g)].map((m) => m[1]);
}
const headerText = (html) => {
  const start = html.indexOf('<header class="week-header ');
  return html.slice(start, html.indexOf('</header>', start));
};
const headerHrefs = (html) => hrefsIn(html, '<header class="week-header ', '</header>');
const usedIn = (html) => hrefsIn(html, '<p class="used-in-weeks ', '</p>');
const readWith = (html) => hrefsIn(html, '<nav class="read-with ', '</nav>');

// Which weeks use each track, from the week sources. Independent of src/lib/links.ts.
const expectedWeeks = (slug) =>
  weeks.filter((w) => frontmatter(join(weeksDir, `${w.slug}.mdx`)).tracks.includes(slug)).map((w) => w.n);

test('the networking track page lists weeks 4 and 12 with links', () => {
  const html = page(dist, trackPath('networking'));
  assert.match(html, /Used in weeks/);
  assert.deepEqual(usedIn(html), [`${base}${weekPath(4)}`, `${base}${weekPath(12)}`]);
});

test('week 4 links to the networking, dns, tls-pki and time-ntp tracks', () => {
  const html = page(dist, weekPath(4));
  assert.deepEqual(readWith(html), ['networking', 'dns', 'tls-pki', 'time-ntp'].map((s) => `${base}${trackPath(s)}`));
});

test('every track page links exactly the weeks whose frontmatter names it, ascending', () => {
  for (const t of tracks) {
    const want = expectedWeeks(t.slug);
    assert.ok(want.length > 0, `${t.slug}: no week uses this track`);
    assert.deepEqual(usedIn(page(dist, trackPath(t.slug))), want.map((n) => `${base}${weekPath(n)}`), t.slug);
  }
});

test('every week page links exactly the tracks in its frontmatter, in order', () => {
  for (const w of weeks) {
    const want = frontmatter(join(weeksDir, `${w.slug}.mdx`)).tracks;
    assert.deepEqual(readWith(page(dist, weekPath(w.n))), want.map((s) => `${base}${trackPath(s)}`), w.slug);
  }
});

test('week header shows phase, hours as a range, AI stage and the previous week', () => {
  const html = page(dist, weekPath(4));
  const header = headerText(html);
  const [low, high] = frontmatter(join(weeksDir, '04-networking.mdx')).hours;
  assert.match(header, /Foundations/);
  assert.ok(header.includes(`${low}–${high} hours`), `header should show ${low}–${high} hours`);
  assert.match(header, /Explain only/);
  assert.deepEqual(headerHrefs(html), [`${base}${weekPath(3)}`]);
});

test('week 0 has no previous-week link and week 18 uses the pair-partner stage', () => {
  assert.deepEqual(headerHrefs(page(dist, weekPath(0))), []);
  assert.match(page(dist, weekPath(18)), /Pair partner/);
  assert.match(page(dist, weekPath(8)), /Review everything/);
});

test('every week page renders the header, three callouts, a side lab and a Read with list', () => {
  for (const w of weeks) {
    const html = page(dist, weekPath(w.n));
    assert.equal((html.match(/<header class="week-header /g) ?? []).length, 1, `${w.slug}: week header`);
    for (const cls of ['callout-observe', 'callout-operate', 'callout-ai']) {
      assert.equal((html.match(new RegExp(`class="callout ${cls}"`, 'g')) ?? []).length, 1, `${w.slug}: ${cls}`);
    }
    assert.match(html, /<details class="side-lab"/, `${w.slug}: side lab`);
  }
});

test('the callout CSS uses only Starlight variables, so the theme toggle recolours it', () => {
  const css = readFileSync(join(root, 'src/styles/custom.css'), 'utf8');
  assert.doesNotMatch(css, /#[0-9a-fA-F]{3,8}\b|rgba?\(|hsla?\(/, 'no literal colours');
  assert.match(css, /var\(--sl-color-/);
});

// Build into a temp dir with other bases. BASE_URL may or may not end in a slash.
for (const customBase of ['/fsp/', '/fsp']) {
  test(`BASE_PATH=${customBase}: week and track links start with /fsp/ and have no doubled slash`, { timeout: 300_000 }, () => {
    const outDir = mkdtempSync(join(tmpdir(), 'links-base-'));
    try {
      const r = spawnSync('npm', ['run', 'build', '--', '--outDir', outDir], {
        cwd: root,
        encoding: 'utf8',
        env: { ...process.env, NO_COLOR: '1', BASE_PATH: customBase },
      });
      assert.equal(r.status, 0, r.stdout + r.stderr);
      const b = normalizeBase(customBase);
      assert.equal(b, '/fsp/');
      const track = page(outDir, trackPath('networking'));
      assert.deepEqual(usedIn(track), [`/fsp/${weekPath(4)}`, `/fsp/${weekPath(12)}`]);
      const week = page(outDir, weekPath(4));
      assert.deepEqual(readWith(week), ['networking', 'dns', 'tls-pki', 'time-ntp'].map((s) => `/fsp/${trackPath(s)}`));
      assert.deepEqual(headerHrefs(week), [`/fsp/${weekPath(3)}`]);
      for (const h of [...usedIn(track), ...readWith(week)]) assert.doesNotMatch(h, /\/\//, `${h} has a doubled slash`);
    } finally {
      rmSync(outDir, { recursive: true, force: true });
    }
  });
}
