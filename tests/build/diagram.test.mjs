import { test } from 'node:test';
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../../', import.meta.url));
const html = readFileSync(join(root, 'dist/dev/diagrams/index.html'), 'utf8');
const figures = [...html.matchAll(/<figure class="dg"[^>]*>([\s\S]*?)<\/figure>/g)].map((m) => m[0]);

test('fixture page has two inline-SVG diagram figures', () => {
  assert.equal(figures.length, 2, `expected 2 figure.dg, got ${figures.length}`);
  for (const f of figures) {
    assert.match(f, /<svg[\s>]/, 'figure without inline <svg>');
    assert.doesNotMatch(f, /<img[\s>]/, 'figure contains <img>');
    assert.match(f, /<figcaption>[^<]+<\/figcaption>/, 'figure without figcaption title');
  }
  assert.doesNotMatch(html, /<picture[\s>]/, 'page contains <picture>');
});

test('flowchart figure has a step list with data-hl keys that exist in the SVG', () => {
  const flow = figures.find((f) => /<ol class="dg-steps"/.test(f));
  assert.ok(flow, 'no <ol class="dg-steps"> inside a figure');
  const hls = [...flow.matchAll(/<li data-hl="([^"]*)"/g)].map((m) => m[1]);
  assert.ok(hls.length >= 2, 'expected at least two steps');
  const keys = new Set([...flow.matchAll(/data-k="([^"]+)"/g)].map((m) => m[1].replaceAll('&gt;', '>')));
  for (const hl of hls) {
    for (const k of hl.replaceAll('&gt;', '>').split(/\s+/).filter(Boolean)) {
      assert.ok(keys.has(k), `step key ${k} has no data-k element`);
    }
  }
});

test('built CSS maps D2 theme variables to Starlight variables', () => {
  const dir = join(root, 'dist/_astro');
  const css = readdirSync(dir)
    .filter((f) => f.endsWith('.css'))
    .map((f) => readFileSync(join(dir, f), 'utf8'))
    .join('\n');
  assert.match(css, /--d2-B1:\s*var\(--sl-color-/);
  assert.match(css, /--d2-N1:\s*var\(--sl-color-/);
  assert.match(css, /--d2-N7:\s*var\(--sl-color-/);
});

test('an unknown step key fails the build and names the key', { timeout: 300_000 }, () => {
  const fixture = join(root, 'src/content/docs/dev/zz-bad-diagram.mdx');
  const outDir = mkdtempSync(join(tmpdir(), 'dg-bad-'));
  writeFileSync(
    fixture,
    `---
title: Bad diagram
---
import Diagram from '../../../components/Diagram.astro';

<Diagram id="bad" title="Bad" code={\`a -> b\`} steps={[{ hl: ['nope'], caption: 'x' }]} />
`,
  );
  try {
    const r = spawnSync('npm', ['run', 'build', '--', '--outDir', outDir], { cwd: root, encoding: 'utf8' });
    assert.notEqual(r.status, 0, 'build should fail');
    const out = r.stdout + r.stderr;
    assert.match(out, /nope/, 'error should name the missing key');
    assert.match(out, /\bbad\b/, 'error should name the diagram id');
  } finally {
    rmSync(fixture, { force: true });
    rmSync(outDir, { recursive: true, force: true });
  }
});
