import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { weeks } from '../../src/data/weeks.ts';
import { tracks } from '../../src/data/tracks.ts';
import { skipBraces, skipString, skipTag } from '../lib/prose.mjs';
import { base, frontmatter, root, tracksDir, weeksDir } from './helpers.mjs';

// The track page layout from the design spec, checked on every written track page.
// Every track page is written; a page that still holds the stub sentence fails.
// The source checks read src/; the link check reads dist/.
//
// Headings, components and links are read outside code: fenced code blocks, inline
// code and the strings and template literals inside JSX props (D2 code holds `#`
// comments and `<br/>`) never count as a section, a component or a link.

const STUB = 'This section is not written yet.';
const SECTIONS = ['What It Is', 'Diagrams', 'Example', 'In the Uptime Monitor', 'Common Mistakes', 'Going Deeper', 'Further Reading'];

const source = (t) => readFileSync(join(tracksDir, t.group, `${t.slug}.mdx`), 'utf8');
const written = tracks.filter((t) => !source(t).includes(STUB));

/** Which weeks use a track, from the `tracks: [a, b]` list in each week's frontmatter. */
function weeksFor(slug) {
  return weeks
    .filter((w) => {
      const fm = frontmatter(join(weeksDir, `${w.slug}.mdx`));
      assert.ok(Array.isArray(fm.tracks), `weeks/${w.slug}.mdx: tracks must be a flow list, tracks: [a, b]; the test reads no other form`);
      return fm.tracks.includes(slug);
    })
    .map((w) => w.n);
}

const blank = (s) => s.replace(/[^\n]/g, ' ');
const escapeRe = (s) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

/**
 * The attributes of one JSX opening tag, in any order and over any number of lines:
 * a Map of name to the raw value as written ("x", {…}), or true for a bare attribute.
 */
function attributes(tag, from) {
  const attrs = new Map();
  let j = from;
  while (j < tag.length) {
    const c = tag[j];
    if (c === '>') break;
    if (c === '{') {
      j = skipBraces(tag, j); // a spread, {...props}
      continue;
    }
    const name = /^[A-Za-z_:][\w:.-]*/.exec(tag.slice(j));
    if (!name) {
      j++;
      continue;
    }
    j += name[0].length;
    let k = j;
    while (/\s/.test(tag[k] ?? '')) k++;
    let value = true;
    if (tag[k] === '=') {
      k++;
      while (/\s/.test(tag[k] ?? '')) k++;
      const end = tag[k] === '{' ? skipBraces(tag, k) : tag[k] === '"' || tag[k] === "'" ? skipString(tag, k) : k;
      value = tag.slice(k, end);
      j = end;
    }
    attrs.set(name[0], value);
  }
  return attrs;
}

/** A string prop: "x", 'x', {'x'}, {"x"} or {`x`}. */
const stringProp = (raw) =>
  typeof raw === 'string' ? /^\{?\s*(["'`])((?:\\.|(?!\1)[^\\])*)\1\s*\}?$/.exec(raw)?.[2] : undefined;

/** A number prop: {4} or "4". */
function numberProp(raw) {
  const m = typeof raw === 'string' && /^(?:\{\s*(\d+)\s*\}|(["'])(\d+)\2)$/.exec(raw);
  return m ? Number(m[1] ?? m[3]) : undefined;
}

/**
 * Reads an .mdx source once, keeping offsets. Returns
 *   flat  the source with frontmatter, fenced code, inline code, JSX tags and {expressions} blanked;
 *   tags  [{ name, start, attrs }] for every JSX opening tag outside code.
 * The fence rule is the one tests/lib/prose.mjs uses.
 */
function scan(src) {
  let text = src;
  const fm = /^---\n[\s\S]*?\n---(?=\n|$)/.exec(text);
  if (fm) text = blank(fm[0]) + text.slice(fm[0].length);

  const lines = text.split('\n');
  let fence = null;
  for (let i = 0; i < lines.length; i++) {
    const open = /^\s*(`{3,}|~{3,})/.exec(lines[i]);
    if (fence) {
      if (open && open[1][0] === fence[0] && open[1].length >= fence.length && /^\s*[`~]+\s*$/.test(lines[i])) fence = null;
      lines[i] = blank(lines[i]);
    } else if (open) {
      fence = open[1];
      lines[i] = blank(lines[i]);
    }
  }
  text = lines.join('\n');

  const tags = [];
  let flat = '';
  let i = 0;
  while (i < text.length) {
    const c = text[i];
    let end = i + 1;
    let hide = false;
    if (c === '<' && /[A-Za-z]/.test(text[i + 1] ?? '')) {
      end = skipTag(text, i);
      const tag = text.slice(i, end);
      const name = /^<([A-Za-z][\w.:-]*)/.exec(tag)[1];
      tags.push({ name, start: i, attrs: attributes(tag, 1 + name.length) });
      hide = true;
    } else if (c === '<' && text[i + 1] === '/' && /[A-Za-z]/.test(text[i + 2] ?? '')) {
      end = skipTag(text, i);
      hide = true;
    } else if (c === '{') {
      end = skipBraces(text, i);
      hide = true;
    } else if (c === '`') {
      const run = /^`+/.exec(text.slice(i))[0];
      const close = text.indexOf(run, i + run.length);
      if (close !== -1 && text[close + run.length] !== '`') {
        end = close + run.length;
        hide = true;
      } else {
        end = i + run.length;
      }
    } else if (c === '\\') {
      end = i + 2;
    }
    const chunk = text.slice(i, end);
    flat += hide ? blank(chunk) : chunk;
    i = end;
  }
  return { flat, tags };
}

/** The `## ` sections in order: [{ heading, start, end }], offsets of the body in the source. Duplicates stay. */
function sections(src, flat) {
  const found = [...flat.matchAll(/^## (.+)$/gm)];
  return found.map((m, k) => ({
    heading: src.slice(m.index + 3, m.index + m[0].length).trim(),
    start: m.index + m[0].length,
    end: k + 1 < found.length ? found[k + 1].index : src.length,
  }));
}

function parse(t) {
  const src = source(t);
  const { flat, tags } = scan(src);
  const s = sections(src, flat);
  const tagsIn = (heading) => {
    const sec = s.find((x) => x.heading === heading);
    return sec ? tags.filter((g) => g.start >= sec.start && g.start < sec.end) : [];
  };
  const flatIn = (heading) => {
    const sec = s.find((x) => x.heading === heading);
    return sec ? flat.slice(sec.start, sec.end) : '';
  };
  return { src, tags, sections: s, tagsIn, flatIn };
}

test('every track page is written', () => {
  const stubs = tracks.filter((t) => source(t).includes(STUB)).map((t) => `${t.group}/${t.slug}`);
  assert.deepEqual(stubs, [], `pages still hold "${STUB}"`);
});

for (const t of written) {
  const name = `${t.group}/${t.slug}.mdx`;

  test(`track ${name}: frontmatter, sections and components`, () => {
    const { src, tags, sections: s } = parse(t);
    const fm = frontmatter(join(tracksDir, t.group, `${t.slug}.mdx`));
    assert.equal(typeof fm.description, 'string', `${name}: description is required`);
    assert.ok(
      !/^[>|][-+]?\d*$/.test(fm.description),
      `${name}: description is a block scalar (${fm.description}); write it on one line, the test reads only one-line values`,
    );
    assert.ok(fm.description.length > 20, `${name}: description is too short`);

    const headings = s.map((x) => x.heading);
    const duplicates = headings.filter((h, k) => headings.indexOf(h) !== k);
    assert.deepEqual(duplicates, [], `${name}: duplicate ## sections: ${duplicates.join(', ')}`);
    assert.deepEqual(headings, SECTIONS, `${name}: the ## sections must be exactly these, in order`);
    for (const x of s) assert.ok(src.slice(x.start, x.end).trim().length > 0, `${name}: ${x.heading} is empty`);

    const named = (n) => tags.filter((g) => g.name === n);
    assert.ok(named('UsedInWeeks').length > 0, `${name}: <UsedInWeeks /> is missing`);
    const diagrams = named('Diagram');
    assert.ok(diagrams.length >= 2, `${name}: needs at least two diagrams, has ${diagrams.length}`);
    assert.ok(
      diagrams.some((d) => typeof d.attrs.get('steps') === 'string' && d.attrs.get('steps').startsWith('{')),
      `${name}: at least one diagram needs a walkthrough, steps={[…]}`,
    );
    assert.ok(
      named('CodeFrom').some((g) => /^(?:go\/tracks\/|web\/src\/tracks\/)/.test(stringProp(g.attrs.get('file')) ?? '')),
      `${name}: needs a <CodeFrom> from examples/go/tracks or examples/web/src/tracks`,
    );
  });

  test(`track ${name}: In the Uptime Monitor links every week that uses it`, () => {
    const { tagsIn } = parse(t);
    const linked = new Set(tagsIn('In the Uptime Monitor').filter((g) => g.name === 'WeekLink').map((g) => numberProp(g.attrs.get('n'))));
    for (const n of weeksFor(t.slug)) assert.ok(linked.has(n), `${name}: no <WeekLink n={${n}}> in In the Uptime Monitor`);
  });

  test(`track ${name}: Further Reading has three external links`, () => {
    const { flatIn } = parse(t);
    const links = new Set([...flatIn('Further Reading').matchAll(/\]\(\s*(https:\/\/[^\s)]+)/g)].map((m) => m[1]));
    assert.ok(links.size >= 3, `${name}: Further Reading has ${links.size} distinct https links, needs 3`);
  });

  test(`track ${name}: every internal link and anchor resolves in dist/`, () => {
    const page = (path) => join(root, 'dist', path, 'index.html');
    const idOn = (html, id) => new RegExp(`\\sid="${escapeRe(id)}"`).test(html);
    const file = page(`tracks/${t.group}/${t.slug}`);
    assert.ok(existsSync(file), `${name}: ${file} is missing; run the build first`);
    const html = readFileSync(file, 'utf8');
    // Only the page content: the sidebar and header links are tested elsewhere.
    const open = html.indexOf('<main');
    const close = html.indexOf('</main>', open);
    assert.ok(open !== -1 && close !== -1, `${name}: the built page has no <main> element`);
    const main = html.slice(open, close);
    const hrefs = [...new Set([...main.matchAll(/\shref="([^"]+)"/g)].map((m) => m[1]))];
    const internal = hrefs.filter((h) => h.startsWith(`${base}weeks/`) || h.startsWith(`${base}tracks/`));
    assert.ok(
      internal.length > 0,
      `${name}: no link to a week or track page under ${base} in <main>; every written page links its weeks. Was dist/ built with BASE_PATH=${base}?`,
    );
    for (const href of internal) {
      const [path, anchor] = href.slice(base.length).split('#');
      const target = page(path.replace(/\/$/, ''));
      assert.ok(existsSync(target), `${name}: ${href} names no built page`);
      if (anchor) assert.ok(idOn(readFileSync(target, 'utf8'), anchor), `${name}: ${href}: no id="${anchor}" on that page`);
    }
    for (const href of hrefs.filter((h) => h.startsWith('#') && h.length > 1)) {
      assert.ok(idOn(html, href.slice(1)), `${name}: ${href}: no id="${href.slice(1)}" on this page`);
    }
  });

  test(`track ${name}: no id is used twice in the page content`, () => {
    // A Diagram id equal to a heading slug sends the heading anchor to the figure.
    const html = readFileSync(join(root, 'dist', 'tracks', t.group, t.slug, 'index.html'), 'utf8');
    const open = html.indexOf('<main');
    const close = html.indexOf('</main>', open);
    assert.ok(open !== -1 && close !== -1, `${name}: the built page has no <main> element`);
    const ids = [...html.slice(open, close).matchAll(/\sid="([^"]+)"/g)].map((m) => m[1]);
    assert.ok(ids.length > 0, `${name}: no id attributes in <main>`);
    const duplicates = [...new Set(ids.filter((id, k) => ids.indexOf(id) !== k))];
    assert.deepEqual(duplicates, [], `${name}: ids used more than once in <main>: ${duplicates.join(', ')}`);
  });
}
