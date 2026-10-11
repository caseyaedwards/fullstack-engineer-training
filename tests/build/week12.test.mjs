import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { base, root } from './helpers.mjs';

// Week 12 hands the reader a patch with four planted bugs and fixes each one with
// a tool and a regression test. These checks keep the page complete, keep the
// download command pointing at the tested patch, and keep the four fixes and their
// tests on the page.

const html = readFileSync(join(root, 'dist/weeks/12-debugging/index.html'), 'utf8');
const figures = [...html.matchAll(/<figure class="dg"[^>]*>[\s\S]*?<\/figure>/g)].map((m) => m[0]);
// Expressive Code splits tokens into spans, so compare code on its text with tags removed.
const named = { amp: '&', lt: '<', gt: '>', quot: '"', apos: "'" };
const unescape = (s) =>
  s.replace(/&(?:#x([0-9a-f]+)|#(\d+)|(\w+));/gi, (m, hex, dec, name) =>
    hex ? String.fromCodePoint(parseInt(hex, 16)) : dec ? String.fromCodePoint(Number(dec)) : (named[name] ?? m));
const blocks = (lang) =>
  [...html.matchAll(new RegExp(`<pre data-language="${lang}">([\\s\\S]*?)</pre>`, 'g'))].map((m) =>
    unescape(m[1].replace(/<div class="ec-line[^"]*">/g, '\n').replace(/<[^>]+>/g, '')).replace(/^\n/, ''),
  );

test('week 12 has at least three inline-SVG diagrams, each with walkthrough steps', () => {
  assert.ok(figures.length >= 3, `expected >= 3 figure.dg, got ${figures.length}`);
  for (const f of figures) assert.match(f, /<svg[\s>]/, 'figure without inline <svg>');
  for (const id of ['tools', 'breakpoint', 'browser-cache']) {
    assert.ok(figures.some((f) => f.includes(`id="${id}"`) && f.includes('dg-steps')), `diagram ${id} has no walkthrough`);
  }
});

test('the download command fetches the tested patch, under the site base', () => {
  const sh = blocks('sh').join('\n');
  const m = /curl -fsSLo \/tmp\/teammate\.patch (\S+)\ngit am \/tmp\/teammate\.patch/.exec(sh);
  assert.ok(m, `no curl and git am pair for the patch in:\n${sh}`);
  const url = new URL(m[1]);
  assert.ok(url.pathname.startsWith(base), `${url.pathname} is not under the site base ${base}`);
  const file = join(root, 'dist', url.pathname.slice(base.length));
  assert.ok(existsSync(file), `${url.pathname} is not in dist/`);
  const patch = readFileSync(join(root, 'examples/go/week12/testdata/teammate.patch'), 'utf8');
  assert.equal(readFileSync(file, 'utf8'), patch, 'the served patch differs from examples/go/week12/testdata/teammate.patch');
  assert.equal((patch.match(/^Subject: \[PATCH \d\/4\]/gm) ?? []).length, 4, 'the patch must hold the four commits');
});

test('week 12 shows each fix with its regression test', () => {
  const go = blocks('go').join('\n');
  for (const name of ['TestSecureHeaders', 'TestAnswersAreNotStored', 'TestIdleWorkersWaitForPoll', 'TestTLSRowIsTheHandshakeOnly']) {
    assert.match(go, new RegExp(`func ${name}\\(t \\*testing\\.T\\)`), `${name} is not on the page`);
  }
  assert.match(go, /sent := rec\.Result\(\)\.Header/, 'the headers test must read what the client receives');
  assert.match(go, /h\.Set\("Content-Security-Policy", "frame-ancestors 'none'"\)\n\s*next\.ServeHTTP\(w, r\)/, 'the headers must be set before next runs');
  assert.match(go, /w\.Header\(\)\.Set\("Cache-Control", "no-store"\)/);
  assert.match(go, /end\(&t\.TLS, &tlsStart\)/);
  assert.doesNotMatch(go, /end\(&t\.TLS, &connectStart\)|max-age=300/, 'the planted code must not appear as a fix');
});

test('the side lab bisects the worker with a check that builds at every commit', () => {
  const sh = blocks('sh').join('\n');
  assert.match(sh, /git bisect run sh -c 'go run \.\/cmd\/worker -h 2>&1 \| grep -q "\(default 1s\)"'/);
  // poll_test.go does not compile before the worker commit, so bisecting with it names the wrong commit.
  assert.doesNotMatch(html, /Repeat it with <code[^>]*>TestIdleWorkersWaitForPoll/);
});

test('week 12 links to the weeks whose code it debugs, with the site base', () => {
  const hrefs = new Set([...html.matchAll(/href="([^"]+)"/g)].map((m) => m[1]));
  for (const slug of ['04-networking', '05-concurrency', '09-jobs-alerts', '10-frontend', '11-security-auth', '13-observability']) {
    const href = `${base}weeks/${slug}/`;
    assert.ok(hrefs.has(href), `no link to ${href}`);
  }
});

test('week 12 has every standard box and no stub text', () => {
  for (const marker of ['callout callout-observe', 'callout callout-operate', 'callout callout-ai', 'side-lab', 'done-when']) {
    assert.match(html, new RegExp(`class="${marker}[ "]`), `missing ${marker}`);
  }
  assert.doesNotMatch(html, /not written yet|coming soon/i);
});

test('week 12 has four to six done items with kebab-case ids', () => {
  const ids = [...html.matchAll(/type="checkbox" value="([^"]+)"/g)].map((m) => m[1]);
  assert.ok(ids.length >= 4 && ids.length <= 6, `expected 4-6 done items, got ${ids.length}`);
  for (const id of ids) {
    assert.match(id, /^[a-z0-9]+(?:-[a-z0-9]+)*$/, `done id ${id} is not kebab-case`);
    assert.doesNotMatch(id, /^item-\d+$/, `done id ${id} is still a placeholder`);
  }
});
