// Shared by the build tests. Reads the MDX sources, not dist/, so a test can
// compare what was authored with what was rendered.
import { readFileSync, readdirSync } from 'node:fs';
import { join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

export const root = fileURLToPath(new URL('../../', import.meta.url));
export const docsDir = join(root, 'src/content/docs');
export const weeksDir = join(docsDir, 'weeks');
export const tracksDir = join(docsDir, 'tracks');

/** Site base with exactly one trailing slash, as BASE_PATH is set in CI. */
export const normalizeBase = (b = '/') => b.replace(/\/?$/, '/');
export const base = normalizeBase(process.env.BASE_PATH);

/** Every .mdx file under dir, as paths relative to dir, sorted. */
export function mdxFiles(dir) {
  const out = [];
  const walk = (d) => {
    for (const e of readdirSync(d, { withFileTypes: true })) {
      if (e.isDirectory()) walk(join(d, e.name));
      else if (e.name.endsWith('.mdx')) out.push(relative(dir, join(d, e.name)));
    }
  };
  walk(dir);
  return out.sort();
}

const scalar = (s) => {
  const t = s.trim();
  if (/^-?\d+(\.\d+)?$/.test(t)) return Number(t);
  return t.replace(/^(["'])(.*)\1$/, '$2');
};

/**
 * Top-level frontmatter keys only. Handles the shapes the stubs use: scalars and
 * flow lists like `tracks: [a, b]`. Nested blocks (e.g. `sidebar:`) come back as ''.
 */
export function frontmatter(file) {
  const m = readFileSync(file, 'utf8').match(/^---\n([\s\S]*?)\n---/);
  if (!m) throw new Error(`${file} has no frontmatter`);
  const data = {};
  for (const line of m[1].split('\n')) {
    const kv = line.match(/^([A-Za-z][\w-]*):(.*)$/);
    if (!kv) continue;
    const v = kv[2].trim();
    data[kv[1]] = v.startsWith('[') ? v.slice(1, -1).split(',').map(scalar).filter((x) => x !== '') : scalar(v);
  }
  return data;
}
