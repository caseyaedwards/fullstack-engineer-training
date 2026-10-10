import { test } from 'node:test';
import assert from 'node:assert/strict';
import { join } from 'node:path';
import { weeks } from '../../src/data/weeks.ts';
import { tracks } from '../../src/data/tracks.ts';
import { frontmatter, mdxFiles, tracksDir, weeksDir } from './helpers.mjs';

// These read the sources, not dist/. A typo in a key name (`wek:`) is not a schema
// error, because every week field is optional until `week` is set. The page would
// quietly build as a non-week page, so check the keys here.

test('every file under weeks/ has a week key (a wek: typo would silently make it a plain page)', () => {
  const files = mdxFiles(weeksDir);
  assert.equal(files.length, weeks.length);
  for (const f of files) {
    const fm = frontmatter(join(weeksDir, f));
    assert.equal(typeof fm.week, 'number', `weeks/${f}: missing or non-numeric "week" key (found keys: ${Object.keys(fm).join(', ')})`);
  }
});

test('each week file matches the registry: week number, filename slug, phase and title', () => {
  for (const w of weeks) {
    const fm = frontmatter(join(weeksDir, `${w.slug}.mdx`));
    assert.equal(fm.week, w.n, `${w.slug}: week is ${fm.week}, registry says ${w.n}`);
    assert.equal(fm.phase, w.phase, `${w.slug}: phase is "${fm.phase}", registry says "${w.phase}"`);
    // "Week N: " keeps week titles apart from track titles such as "Networking" in tabs and search.
    // The sidebar and timeline use the registry title, so the page title must not drift from it.
    assert.equal(fm.title, `Week ${w.n}: ${w.title}`, `${w.slug}: title`);
  }
});

test('each track page has track equal to its filename slug and sits in its registry group directory', () => {
  const files = mdxFiles(tracksDir);
  assert.equal(files.length, tracks.length);
  for (const t of tracks) {
    const rel = `${t.group}/${t.slug}.mdx`;
    assert.ok(files.includes(rel), `registry says tracks/${rel} should exist`);
    const fm = frontmatter(join(tracksDir, rel));
    assert.equal(fm.track, t.slug, `tracks/${rel}: track is "${fm.track}", expected "${t.slug}"`);
  }
  // And the reverse: no page outside the registry's layout.
  const expected = new Set(tracks.map((t) => `${t.group}/${t.slug}.mdx`));
  for (const f of files) assert.ok(expected.has(f), `tracks/${f} is not in the registry (wrong group directory or unknown slug)`);
});
