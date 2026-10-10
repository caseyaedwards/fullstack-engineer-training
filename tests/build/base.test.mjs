import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const html = readFileSync(new URL('../../dist/index.html', import.meta.url), 'utf8');
const base = process.env.BASE_PATH ?? '/';

test('home page title contains the site name', () => {
  const m = html.match(/<title>([^<]*)<\/title>/);
  assert.ok(m, 'no <title> found');
  assert.match(m[1], /Full-Stack Engineer Path/);
});

test('every root-relative href/src respects BASE_PATH', () => {
  const prefix = base.endsWith('/') ? base : `${base}/`;
  const urls = [...html.matchAll(/(?:href|src)="(\/[^"]*)"/g)].map((m) => m[1]);
  assert.ok(urls.length > 0, 'no root-relative URLs found');
  for (const u of urls) {
    if (u.startsWith('//')) continue; // protocol-relative, not site-local
    assert.ok(u.startsWith(prefix), `${u} does not start with ${prefix}`);
  }
});
