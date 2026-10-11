import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { base, root } from './helpers.mjs';

const dist = join(root, 'dist');
const pages = ['', 'start-here/', 'weeks/04-networking/'];
const read = (p) => readFileSync(join(dist, p, 'index.html'), 'utf8');
const head = (html) => html.slice(html.indexOf('<head'), html.indexOf('</head>'));

test('the header holds a labelled width select: Slim, Medium, Wide, Full, Medium selected', () => {
  for (const p of pages) {
    const html = read(p);
    const m = html.match(/<starlight-width-select[\s\S]*?<\/starlight-width-select>/);
    assert.ok(m, `${p}: no width select`);
    assert.match(m[0], /Page width/);
    const opts = [...m[0].matchAll(/<option value="([^"]+)"([^>]*)>([^<]*)<\/option>/g)];
    assert.deepEqual(
      opts.map((o) => [o[1], o[3]]),
      [['slim', 'Slim'], ['medium', 'Medium'], ['wide', 'Wide'], ['full', 'Full']],
    );
    const selected = opts.filter((o) => /\bselected\b/.test(o[2]));
    assert.deepEqual(selected.map((o) => o[1]), ['medium']);
  }
});

test('the theme select is still rendered beside the width select', () => {
  assert.match(read('weeks/04-networking/'), /<starlight-theme-select>/);
});

test('an inline head script applies fsp-width as data-width, guarded by try/catch', () => {
  for (const p of pages) {
    const h = head(read(p));
    const scripts = [...h.matchAll(/<script(?![^>]*\bsrc=)[^>]*>([\s\S]*?)<\/script>/g)].map((m) => m[1]);
    const s = scripts.find((x) => x.includes('fsp-width'));
    assert.ok(s, `${p}: no inline fsp-width script in head`);
    assert.match(s, /try\s*\{/);
    assert.match(s, /catch/);
    assert.match(s, /data-width|dataset\.width/);
  }
});

test('the width CSS sets --sl-content-width per data-width and uses no literal colours', () => {
  const css = readFileSync(join(root, 'src/styles/width.css'), 'utf8');
  for (const w of ['slim', 'wide', 'full']) assert.match(css, new RegExp(`data-width=['"]?${w}`));
  assert.match(css, /--sl-content-width/);
  assert.doesNotMatch(css, /#[0-9a-fA-F]{3,8}\b|rgba?\(|hsla?\(/);
});
