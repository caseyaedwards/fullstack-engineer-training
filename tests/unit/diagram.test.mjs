import { test } from 'node:test';
import assert from 'node:assert/strict';
import { decodeKey, normalizeKey } from '../../src/lib/diagram/keys.mjs';
import { renderD2 } from '../../src/lib/diagram/render.mjs';

const b64 = (s) => Buffer.from(s, 'utf8').toString('base64');

const FLOW = `browser -> resolver -> root
resolver -> tld: cache miss
browser -> server
`;

test('decodeKey: shapes, nested shapes and edges', () => {
  assert.equal(decodeKey(b64('browser')), 'browser');
  assert.equal(decodeKey(b64('vpc.api')), 'vpc.api');
  assert.equal(decodeKey(b64('(browser -&gt; dns)[0]')), 'browser->dns');
  assert.equal(decodeKey(b64('(b -&gt; s)[1]')), 'b->s#1');
});

test('decodeKey: edges inside containers are fully qualified', () => {
  assert.equal(decodeKey(b64('vpc.(api -&gt; db)[0]')), 'vpc.api->vpc.db');
});

test('decodeKey: non-key classes yield null', () => {
  for (const c of ['shape', 'connection', 'text', 'd2-123 d2-svg', 'fill-B6', '']) {
    assert.equal(decodeKey(c), null, c);
  }
});

test('normalizeKey: undirected and reverse edges keep their operator', () => {
  assert.equal(normalizeKey('(a -- b)[0]'), 'a--b');
  assert.equal(normalizeKey('(a &lt;- b)[2]'), 'a<-b#2');
});

test('renderD2: keys, themed CSS, data-k and explicit size', async () => {
  const { svg, keys } = await renderD2(FLOW, { id: 'flow' });
  for (const k of ['browser', 'resolver', 'root', 'tld', 'server', 'browser->resolver', 'resolver->tld']) {
    assert.ok(keys.has(k), `missing key ${k}`);
  }
  const rules = svg.match(/\.(?:fill|stroke|color)-[A-Z]+\d\{[^}]*\}/g) ?? [];
  assert.ok(rules.length > 0, 'no theme rules found');
  for (const r of rules) assert.doesNotMatch(r, /#[0-9a-f]{3,6}/i, r);
  assert.match(svg, /var\(--d2-/);
  assert.match(svg, /data-k="browser"/);
  assert.match(svg, /data-k="browser-&gt;resolver"|data-k="browser->resolver"/);
  assert.match(svg, /^<svg[^>]*\swidth="\d+"[^>]*>/);
  assert.match(svg, /^<svg[^>]*\sheight="\d+"[^>]*>/);
});

test('renderD2: sequence messages are keyed in order', async () => {
  const { keys } = await renderD2(
    `shape: sequence_diagram
browser -> server: SYN
server -> browser: SYN-ACK
browser -> server: ACK
`,
    { id: 'seq' },
  );
  for (const k of ['browser', 'server', 'browser->server', 'server->browser', 'browser->server#1']) {
    assert.ok(keys.has(k), `missing key ${k}`);
  }
});

test('renderD2: two ids for the same source share no element ids', async () => {
  const ids = (svg) => new Set([...svg.matchAll(/\sid="([^"]+)"/g)].map((m) => m[1]));
  const a = ids((await renderD2(FLOW, { id: 'a' })).svg);
  const b = ids((await renderD2(FLOW, { id: 'b' })).svg);
  assert.ok(a.size > 0);
  for (const id of a) assert.ok(!b.has(id), `shared id ${id}`);
  for (const id of a) assert.ok(id.startsWith('a-'), `id ${id} not prefixed`);
});

test('renderD2: id references in attributes follow the prefixed ids', async () => {
  const { svg } = await renderD2(FLOW, { id: 'refs' });
  const ids = new Set([...svg.matchAll(/\sid="([^"]+)"/g)].map((m) => m[1]));
  const refs = [...svg.matchAll(/\s(?:[a-z-]+="url\(#([^)]+)\)"|(?:xlink:)?href="#([^"]+)")/g)].map((m) => m[1] ?? m[2]);
  assert.ok(refs.length > 0, 'expected url(#...) references (marker-end, mask)');
  for (const r of refs) assert.ok(ids.has(r) && r.startsWith('refs-'), `dangling or unprefixed ref ${r}`);
});

test('renderD2: invalid D2 rejects with the d2 message', async () => {
  await assert.rejects(renderD2('a -> {', { id: 'bad' }), (err) => {
    assert.match(err.message, /d2/i);
    assert.match(err.message, /err|unexpected|expected/i);
    return true;
  });
});

test('claimId: a repeated diagram id on one page throws, other pages are independent', async () => {
  const { claimId } = await import('../../src/lib/diagram/page-ids.mjs');
  const page1 = new Request('http://x/a/');
  const page2 = new Request('http://x/b/');
  claimId(page1, 'dns');
  claimId(page2, 'dns');
  assert.throws(() => claimId(page1, 'dns'), /"dns".*\/a\//);
});

import { chmodSync, mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { isLifeline } from '../../src/lib/diagram/keys.mjs';

function fakeD2(body) {
  const f = join(mkdtempSync(join(tmpdir(), 'faked2-')), 'd2');
  writeFileSync(f, `#!/bin/sh\n${body}\n`);
  chmodSync(f, 0o755);
  return f;
}

async function withEnv(env, fn) {
  const old = Object.fromEntries(Object.keys(env).map((k) => [k, process.env[k]]));
  Object.assign(process.env, env);
  try {
    return await fn();
  } finally {
    for (const [k, v] of Object.entries(old)) v === undefined ? delete process.env[k] : (process.env[k] = v);
  }
}

test('renderD2: sequence lifelines are not keys and carry no data-k', async () => {
  const { svg, keys } = await renderD2(`shape: sequence_diagram\nbrowser -> server: SYN\n`, { id: 'life' });
  assert.ok(isLifeline('(browser -- )[0]'));
  assert.ok(!isLifeline('browser->server'));
  for (const k of keys) assert.ok(!isLifeline(k), k);
  assert.doesNotMatch(svg, /data-k="\(browser -- \)/);
  assert.ok(keys.has('browser->server'));
});

test('renderD2: d2 exiting before reading stdin rejects with its stderr (no EPIPE crash)', async () => {
  const bin = fakeD2('if [ "$1" = "--version" ]; then echo v0.0.0; exit 0; fi\necho boom-from-d2 >&2\nexit 3');
  const mod = await import('../../src/lib/diagram/render.mjs?epipe');
  await withEnv({ D2_BIN: bin }, async () => {
    await assert.rejects(mod.renderD2('x'.repeat(5_000_000), { id: 'epipe' }), /boom-from-d2/);
  });
});

test('renderD2: a hung d2 is killed and rejects with a timeout error', async () => {
  const bin = fakeD2('if [ "$1" = "--version" ]; then echo v0.0.0; exit 0; fi\nsleep 30');
  const mod = await import('../../src/lib/diagram/render.mjs?timeout');
  await withEnv({ D2_BIN: bin, D2_TIMEOUT_MS: '300' }, async () => {
    const t = Date.now();
    await assert.rejects(mod.renderD2('a -> b', { id: 'hung' }), /timed out after 300 ms/);
    assert.ok(Date.now() - t < 5000);
  });
});

test('renderD2: a failed d2 --version is not cached forever', async () => {
  const mod = await import('../../src/lib/diagram/render.mjs?version');
  await withEnv({ D2_BIN: '/nonexistent/d2' }, async () => {
    await assert.rejects(mod.renderD2('a -> b', { id: 'ver' }), /could not run/);
  });
  const { keys } = await mod.renderD2('a -> b', { id: 'ver' });
  assert.ok(keys.has('a'));
});
