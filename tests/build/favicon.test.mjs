import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { base, root } from './helpers.mjs';

const dist = join(root, 'dist');
const pages = ['', 'start-here/', 'weeks/04-networking/'];
const read = (p) => readFileSync(join(dist, p, 'index.html'), 'utf8');
const head = (html) => html.slice(html.indexOf('<head'), html.indexOf('</head>'));

test('the favicon is built and every page links it under the site base', () => {
  assert.ok(existsSync(join(dist, 'favicon.svg')), 'dist/favicon.svg missing');
  for (const p of pages) {
    const link = head(read(p)).match(/<link[^>]*rel="shortcut icon"[^>]*>/);
    assert.ok(link, `${p}: no favicon link`);
    assert.match(link[0], new RegExp(`href="${base}favicon\\.svg"`), `${p}: ${link[0]}`);
    assert.match(link[0], /type="image\/svg\+xml"/);
  }
});

test('the favicon is clay with a dark-mode rule and no gradient', () => {
  const svg = readFileSync(join(root, 'public/favicon.svg'), 'utf8');
  assert.match(svg, /^<svg[^>]*xmlns="http:\/\/www\.w3\.org\/2000\/svg"/);
  assert.match(svg, /#d97757/i);
  assert.match(svg, /prefers-color-scheme:\s*dark/);
  assert.doesNotMatch(svg, /Gradient/i);
});
