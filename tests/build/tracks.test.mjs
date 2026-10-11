import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { weeks } from '../../src/data/weeks.ts';
import { tracks } from '../../src/data/tracks.ts';
import { base, frontmatter, root, tracksDir, weeksDir } from './helpers.mjs';

// The track page layout from the design spec, checked on every written track page.
// A page that still holds the stub sentence is skipped, so the groups can land one
// PR at a time. The source checks read src/; the link check reads dist/.

const STUB = 'This section is not written yet.';
const SECTIONS = ['What It Is', 'Diagrams', 'Example', 'In the Uptime Monitor', 'Common Mistakes', 'Going Deeper', 'Further Reading'];

const source = (t) => readFileSync(join(tracksDir, t.group, `${t.slug}.mdx`), 'utf8');
const written = tracks.filter((t) => !source(t).includes(STUB));

// Which weeks use each track, from the week sources.
const weeksFor = (slug) =>
  weeks.filter((w) => frontmatter(join(weeksDir, `${w.slug}.mdx`)).tracks.includes(slug)).map((w) => w.n);

/** The body of each `## ` section, by heading text. Fenced code is left in place. */
function sections(src) {
  const body = src.replace(/^---\n[\s\S]*?\n---\n/, '');
  const out = new Map();
  const parts = body.split(/^## (.+)$/m);
  for (let i = 1; i < parts.length; i += 2) out.set(parts[i].trim(), parts[i + 1]);
  return out;
}

test('a page is a full stub or fully written, never a mix', () => {
  for (const t of tracks) {
    const src = source(t);
    const stubs = src.split(STUB).length - 1;
    assert.ok(stubs === 0 || stubs === SECTIONS.length, `${t.group}/${t.slug}: ${stubs} stub sections of ${SECTIONS.length}`);
  }
});

for (const t of written) {
  const name = `${t.group}/${t.slug}.mdx`;

  test(`track ${name}: frontmatter, sections and components`, () => {
    const src = source(t);
    const fm = frontmatter(join(tracksDir, t.group, `${t.slug}.mdx`));
    assert.equal(typeof fm.description, 'string', `${name}: description is required`);
    assert.ok(fm.description.length > 20, `${name}: description is too short`);

    const s = sections(src);
    assert.deepEqual([...s.keys()], SECTIONS, `${name}: the ## sections must be exactly these, in order`);
    for (const [heading, body] of s) assert.ok(body.trim().length > 0, `${name}: ${heading} is empty`);

    assert.match(src, /<UsedInWeeks \/>/, `${name}: <UsedInWeeks /> is missing`);
    const diagrams = src.match(/<Diagram\b[\s\S]*?\/>/g) ?? [];
    assert.ok(diagrams.length >= 2, `${name}: needs at least two diagrams, has ${diagrams.length}`);
    assert.ok(diagrams.some((d) => /\bsteps=\{/.test(d)), `${name}: at least one diagram needs a walkthrough`);
    assert.match(src, /<CodeFrom file="(?:go\/tracks\/|web\/src\/tracks\/)/, `${name}: needs a <CodeFrom> from examples/go/tracks or examples/web/src/tracks`);
  });

  test(`track ${name}: In the Uptime Monitor links every week that uses it`, () => {
    const body = sections(source(t)).get('In the Uptime Monitor') ?? '';
    const linked = new Set([...body.matchAll(/<WeekLink n=\{(\d+)\}/g)].map((m) => Number(m[1])));
    for (const n of weeksFor(t.slug)) assert.ok(linked.has(n), `${name}: no <WeekLink n={${n}}> in In the Uptime Monitor`);
  });

  test(`track ${name}: Further Reading has three external links`, () => {
    const body = sections(source(t)).get('Further Reading') ?? '';
    const links = body.match(/\]\(https:\/\/[^)\s]+\)/g) ?? [];
    assert.ok(links.length >= 3, `${name}: Further Reading has ${links.length} https links, needs 3`);
  });

  test(`track ${name}: every internal link and anchor resolves in dist/`, () => {
    const page = (path) => join(root, 'dist', path, 'index.html');
    const html = readFileSync(page(`tracks/${t.group}/${t.slug}`), 'utf8');
    // Only the page content: the sidebar and header links are tested elsewhere.
    const main = html.slice(html.indexOf('<main'), html.indexOf('</main>'));
    const hrefs = [...main.matchAll(/href="([^"]+)"/g)].map((m) => m[1]);
    const internal = hrefs.filter((h) => h.startsWith(`${base}weeks/`) || h.startsWith(`${base}tracks/`));
    for (const href of internal) {
      const [path, anchor] = href.slice(base.length).split('#');
      const file = page(path.replace(/\/$/, ''));
      assert.ok(existsSync(file), `${name}: ${href} names no built page`);
      if (anchor) {
        assert.ok(readFileSync(file, 'utf8').includes(`id="${anchor}"`), `${name}: ${href}: no id="${anchor}" on that page`);
      }
    }
  });
}
