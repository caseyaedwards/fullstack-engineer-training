import { test } from 'node:test';
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { existsSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../../', import.meta.url));
const weeksDir = join(root, 'src/content/docs/weeks');
const tracksDir = join(root, 'src/content/docs/tracks');
const base = (process.env.BASE_PATH ?? '/').replace(/\/?$/, '/');

const pagesIn = (dir) => readdirSync(dir, { withFileTypes: true }).filter((e) => e.isDirectory() && existsSync(join(dir, e.name, 'index.html')));

test('dist has 19 week pages and 25 track pages', () => {
  assert.equal(pagesIn(join(root, 'dist/weeks')).length, 19);
  const groups = readdirSync(join(root, 'dist/tracks'), { withFileTypes: true }).filter((e) => e.isDirectory());
  const tracks = groups.flatMap((g) => pagesIn(join(root, 'dist/tracks', g.name)));
  assert.equal(tracks.length, 25);
});

test('a typo in a week frontmatter track slug fails the build and names the file and the slug', { timeout: 300_000 }, () => {
  const fixture = join(weeksDir, 'zz-bad-week.mdx');
  const outDir = mkdtempSync(join(tmpdir(), 'schema-bad-'));
  try {
    writeFileSync(
      fixture,
      `---
title: Bad week
week: 3
phase: foundations
hours: [5, 8]
aiStage: 1
tracks: [nope]
done:
  - id: a
    text: A
---

Body.
`,
    );
    const r = spawnSync('npm', ['run', 'build', '--', '--outDir', outDir], { cwd: root, encoding: 'utf8', env: { ...process.env, NO_COLOR: '1' } });
    assert.notEqual(r.status, 0, 'build should fail');
    const out = r.stdout + r.stderr;
    assert.match(out, /zz-bad-week/, 'error should name the file');
    assert.match(out, /nope/, 'error should name the bad slug');
  } finally {
    rmSync(fixture, { force: true });
    rmSync(outDir, { recursive: true, force: true });
  }
});

test('hours with low above high fails the build and says why', { timeout: 300_000 }, () => {
  const fixture = join(weeksDir, 'zz-bad-hours.mdx');
  const outDir = mkdtempSync(join(tmpdir(), 'schema-hours-'));
  try {
    writeFileSync(
      fixture,
      `---
title: Bad hours
week: 3
phase: foundations
hours: [8, 5]
aiStage: 1
tracks: [networking]
done:
  - id: a
    text: A
---

Body.
`,
    );
    const r = spawnSync('npm', ['run', 'build', '--', '--outDir', outDir], { cwd: root, encoding: 'utf8', env: { ...process.env, NO_COLOR: '1' } });
    assert.notEqual(r.status, 0, 'build should fail');
    const out = r.stdout + r.stderr;
    assert.match(out, /zz-bad-hours/, 'error should name the file');
    assert.match(out, /low <= high/, 'error should explain the range rule');
  } finally {
    rmSync(fixture, { force: true });
    rmSync(outDir, { recursive: true, force: true });
  }
});

/** Build with one bad week file and return the combined output; the fixture is always removed. */
function buildWith(name, source) {
  const fixture = join(weeksDir, `${name}.mdx`);
  const outDir = mkdtempSync(join(tmpdir(), 'schema-done-'));
  try {
    writeFileSync(fixture, source);
    const r = spawnSync('npm', ['run', 'build', '--', '--outDir', outDir], { cwd: root, encoding: 'utf8', env: { ...process.env, NO_COLOR: '1' } });
    return { status: r.status, out: r.stdout + r.stderr };
  } finally {
    rmSync(fixture, { force: true });
    rmSync(outDir, { recursive: true, force: true });
  }
}

const weekWith = (done) => `---
title: Bad done
week: 3
phase: foundations
hours: [5, 8]
aiStage: 1
tracks: [networking]
${done}---

Body.
`;

for (const [name, done, expected] of [
  ['zz-no-done', '', /done: Required on week pages/],
  ['zz-empty-done', 'done: []\n', /done: Too small/],
  ['zz-dup-done', 'done:\n  - id: same\n    text: One\n  - id: same\n    text: Two\n', /Duplicate done item id "same"/],
]) {
  test(`${name}: a week with a missing, empty or duplicate-id done list fails the build and names the file`, { timeout: 300_000 }, () => {
    const r = buildWith(name, weekWith(done));
    assert.notEqual(r.status, 0, 'build should fail');
    assert.match(r.out, new RegExp(name), 'error should name the file');
    assert.match(r.out, expected, 'error should name the problem');
  });
}

test('a week number above 18 fails the build and names the file and the field', { timeout: 300_000 }, () => {
  const r = buildWith(
    'zz-week-19',
    `---
title: Week 19
week: 19
phase: run
hours: [5, 8]
aiStage: 3
tracks: [sre]
done:
  - id: a
    text: A
---

Body.
`,
  );
  assert.notEqual(r.status, 0, 'build should fail');
  assert.match(r.out, /zz-week-19/, 'error should name the file');
  assert.match(r.out, /week: Too big.*<=18/, 'error should name the field and the limit');
});

const rawFrontmatter = (file) => {
  const m = readFileSync(file, 'utf8').match(/^---\n([\s\S]*?)\n---/);
  assert.ok(m, `${file} has no frontmatter`);
  return m[1];
};

test('aiStage in each week stub matches the ladder (0-5 -> 1, 6-12 -> 2, 13-18 -> 3)', () => {
  const files = readdirSync(weeksDir).filter((f) => f.endsWith('.mdx'));
  assert.equal(files.length, 19);
  for (const f of files) {
    const fm = rawFrontmatter(join(weeksDir, f));
    const week = Number(fm.match(/^week:\s*(\d+)\s*$/m)?.[1]);
    const stage = Number(fm.match(/^aiStage:\s*(\d)\s*$/m)?.[1]);
    assert.equal(week, Number(f.slice(0, 2)), `${f}: week number does not match the filename`);
    const expected = week <= 5 ? 1 : week <= 12 ? 2 : 3;
    assert.equal(stage, expected, `${f}: aiStage ${stage}, expected ${expected}`);
  }
});

test('every sidebar link on the home page respects BASE_PATH and covers all weeks and tracks', () => {
  const html = readFileSync(join(root, 'dist/start-here/index.html'), 'utf8');
  const hrefs = [...html.matchAll(/<a href="([^"]+)"/g)].map((m) => m[1]);
  const nav = hrefs.filter((h) => /\/(weeks|tracks)\//.test(h));
  assert.equal(nav.filter((h) => h.includes('/weeks/')).length >= 19, true, 'expected 19 week links in the sidebar');
  assert.equal(nav.filter((h) => h.includes('/tracks/')).length >= 25, true, 'expected 25 track links in the sidebar');
  for (const h of nav) assert.ok(h.startsWith(base), `${h} does not start with ${base}`);
  assert.ok(!hrefs.some((h) => h.includes('/dev/')), 'hidden dev fixture page leaked into the sidebar');
});
