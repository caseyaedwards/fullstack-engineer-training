import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { root } from './helpers.mjs';

// Week 11 adds a login to the uptime monitor. These checks keep the page complete,
// keep the security claims on the page in step with the tested example code, and
// check that the migration, the routes and the OpenAPI file agree.

const html = readFileSync(join(root, 'dist/weeks/11-security-auth/index.html'), 'utf8');
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
const monitor = join(root, 'examples/go/week11/your-monitor');
const read = (file) => readFileSync(join(monitor, file), 'utf8');
const text = unescape(html.replace(/<[^>]+>/g, ' '));

test('week 11 has the session and CSRF diagrams, both with walkthrough steps', () => {
  assert.ok(figures.length >= 2, `expected >= 2 figure.dg, got ${figures.length}`);
  for (const f of figures) assert.match(f, /<svg[\s>]/, 'figure without inline <svg>');
  for (const id of ['session-login', 'csrf']) {
    assert.ok(figures.some((f) => f.includes(`id="${id}"`) && f.includes('dg-steps')), `diagram ${id} has no walkthrough`);
  }
});

test('week 11 has every standard box and no stub text', () => {
  for (const marker of ['callout callout-observe', 'callout callout-operate', 'callout callout-ai', 'side-lab', 'done-when']) {
    assert.match(html, new RegExp(`class="${marker}[ "]`), `missing ${marker}`);
  }
  assert.doesNotMatch(html, /not written yet|coming soon/i);
});

test('week 11 has four to six done items with kebab-case ids', () => {
  const ids = [...html.matchAll(/type="checkbox" value="([^"]+)"/g)].map((m) => m[1]);
  assert.ok(ids.length >= 4 && ids.length <= 6, `expected 4-6 done items, got ${ids.length}`);
  for (const id of ids) {
    assert.match(id, /^[a-z0-9]+(?:-[a-z0-9]+)*$/, `done id ${id} is not kebab-case`);
    assert.doesNotMatch(id, /^item-\d+$/, `done id ${id} is still a placeholder`);
  }
});

test('week 11 shows the guarded routes, the cookie attributes and the bcrypt code', () => {
  const go = blocks('go').join('\n');
  assert.match(go, /"POST \/api\/monitors":\s+h\.requireUser\(h\.create\)/);
  assert.match(go, /"DELETE \/api\/monitors\/\{id\}":\s+h\.requireUser\(h\.delete\)/);
  assert.match(go, /"POST \/api\/login":\s+h\.login/);
  assert.match(go, /HttpOnly: true/);
  assert.match(go, /Secure:\s+true/);
  assert.match(go, /SameSite: http\.SameSiteLaxMode/);
  assert.match(go, /bcrypt\.GenerateFromPassword/);
  assert.match(go, /const Cost = 12/);
  assert.match(go, /MaxPasswordBytes = 72/);
  assert.match(go, /s\.Check\(dummyHash, password\)/);
  assert.match(go, /return sameOrigin\(jsonFallback\(mux\)\)/);
  assert.match(go, /case "same-origin", "none":/);
  assert.match(go, /rand\.Text\(\)/);
  const sql = blocks('sql').join('\n');
  assert.match(sql, /CREATE TABLE users/);
  assert.match(sql, /CREATE TABLE sessions/);
  assert.doesNotMatch(sql, /region:/, 'SQL region markers leak onto the page');
});

test('every Set-Cookie line on the page matches the one the api test pins down', () => {
  const pinned = /want := "(session=good-token; [^"]+)"/.exec(read('internal/api/auth_test.go'))[1];
  const attributes = pinned.replace('session=good-token', '');
  const shown = [...blocks('txt').join('\n').matchAll(/Set-Cookie: session=([A-Z2-7]{26})(; [^\n]+)/g)].map((m) => m[2].trim());
  assert.ok(shown.length >= 2, `expected the cookie header on the page at least twice, got ${shown.length}`);
  for (const s of shown) assert.equal(s, attributes, 'a Set-Cookie line on the page differs from the tested cookie');
});

test('the security numbers on the page match the code', () => {
  const service = read('internal/auth/service.go');
  assert.match(service, /SessionTTL = 24 \* time\.Hour/);
  assert.match(service, /MaxAttempts\s+= 5/);
  assert.match(service, /AttemptWindow = 15 \* time\.Minute/);
  assert.match(read('internal/auth/password.go'), /MinPasswordChars = 15/);
  assert.match(service, /MaxEmailBytes = 254/);
  assert.match(read('internal/auth/limit.go'), /const maxKeys = 10_000/);
  assert.match(text, /minimum of 15 characters/);
  assert.match(text, /email over 254 bytes/);
  assert.match(text, /cap of 10,000 keys/);
  assert.match(text, /http\.NewCrossOriginProtection\(\)/);
  assert.match(text, /five login attempts per email address every 15 minutes/);
  assert.match(text, /Max-Age=86400/);
  assert.match(text, /bcrypt at cost 12/);
  for (const name of ['A01:2025 Broken Access Control', 'A04:2025 Cryptographic Failures', 'A05:2025 Injection', 'A07:2025 Authentication Failures', 'A09:2025 Security Logging and Alerting Failures', 'A10:2025 Mishandling of Exceptional Conditions']) {
    assert.ok(text.includes(name), `OWASP category ${name} missing`);
  }
});

test('the migration, the routes and the OpenAPI file agree on the auth routes', () => {
  const migration = read('migrations/003_auth.sql');
  assert.match(migration, /id\s+text PRIMARY KEY/);
  assert.match(migration, /REFERENCES users\(id\) ON DELETE CASCADE/);
  const openapi = read('api/openapi.yaml');
  for (const path of ['/api/login:', '/api/logout:', '/api/me:']) assert.ok(openapi.includes(`  ${path}`), `openapi.yaml lacks ${path}`);
  assert.equal(openapi.match(/\$ref: "#\/components\/responses\/CrossOrigin"/g)?.length, 4, 'every write documents the 403');
  assert.match(openapi, /sessionCookie:\n\s+type: apiKey\n\s+in: cookie\n\s+name: session/);
});
