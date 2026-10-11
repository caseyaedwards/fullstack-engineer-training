import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { base, root } from './helpers.mjs';

// Week 8 stores the monitor in Postgres. These checks keep the page complete, keep
// the migration and the driver version on the page in step with the tested example
// files, and keep the Postgres service that runs those tests in CI.

const html = readFileSync(join(root, 'dist/weeks/08-backend-postgres/index.html'), 'utf8');
const figures = [...html.matchAll(/<figure class="dg"[^>]*>[\s\S]*?<\/figure>/g)].map((m) => m[0]);
// Expressive Code splits tokens into spans and lines into divs: compare code on its text.
const named = { amp: '&', lt: '<', gt: '>', quot: '"', apos: "'" };
const unescape = (s) =>
  s.replace(/&(?:#x([0-9a-f]+)|#(\d+)|(\w+));/gi, (m, hex, dec, name) =>
    hex ? String.fromCodePoint(parseInt(hex, 16)) : dec ? String.fromCodePoint(Number(dec)) : (named[name] ?? m));
const blocks = (lang) =>
  [...html.matchAll(new RegExp(`<pre data-language="${lang}">([\\s\\S]*?)</pre>`, 'g'))].map((m) =>
    unescape(m[1].replace(/<div class="ec-line[^"]*">/g, '\n').replace(/<[^>]+>/g, '')).replace(/^\n/, ''),
  );
const example = (file) => readFileSync(join(root, 'examples/go', file), 'utf8');

test('week 8 has three inline-SVG diagrams, each with walkthrough steps', () => {
  assert.ok(figures.length >= 3, `expected >= 3 figure.dg, got ${figures.length}`);
  for (const f of figures) assert.match(f, /<svg[\s>]/, 'figure without inline <svg>');
  for (const id of ['schema', 'write-path', 'migrate']) {
    assert.ok(figures.some((f) => f.includes(`id="${id}"`) && f.includes('dg-steps')), `diagram ${id} has no walkthrough`);
  }
});

test('week 8 has every standard box and no stub text', () => {
  for (const marker of ['week-header', 'callout callout-observe', 'callout callout-operate', 'callout callout-ai', 'side-lab', 'done-when', 'read-with']) {
    assert.match(html, new RegExp(`class="${marker}[ "]`), `missing ${marker}`);
  }
  assert.doesNotMatch(html, /not written yet/i);
});

test('week 8 has four to six done items with kebab-case ids', () => {
  const ids = [...html.matchAll(/type="checkbox" value="([^"]+)"/g)].map((m) => m[1]);
  assert.ok(ids.length >= 4 && ids.length <= 6, `expected 4-6 done items, got ${ids.length}`);
  for (const id of ids) assert.match(id, /^[a-z0-9]+(?:-[a-z0-9]+)*$/, `done id ${id} is not kebab-case`);
});

test('week 8 shows the migration exactly as the tested 001_init.sql holds it', () => {
  const file = example('week08/your-monitor/migrations/001_init.sql');
  const region = file.split('\n').filter((l) => !/^-- (end)?region: init$/.test(l)).join('\n').trim();
  const shown = blocks('sql').find((b) => b.includes('create table monitors'));
  assert.ok(shown, 'no SQL block with create table monitors');
  // Expressive Code renders an empty line as a line of its own: compare without blank lines.
  const lines = (s) => s.split('\n').filter((l) => l.trim() !== '');
  assert.deepEqual(lines(shown), lines(region));
  for (const want of [/url\s+text not null unique/, /check \(interval_seconds between 30 and 3600\)/, /references monitors \(id\) on delete cascade/, /create index check_results_monitor_time on check_results \(monitor_id, checked_at desc\)/]) {
    assert.match(shown, want);
  }
});

test('week 8 names both pgx lines: latest for the reader, the go.mod version for Go 1.24', () => {
  // One require line, or a line inside a require ( ... ) block once later weeks add modules.
  const required = /^(?:require |\t)github\.com\/jackc\/pgx\/v5 (v\S+)$/m.exec(example('go.mod'))?.[1];
  assert.ok(required, 'examples/go/go.mod does not require github.com/jackc/pgx/v5');
  // The reader's go.mod says Go 1.27 since Week 0: the command block takes the newest pgx.
  const shown = blocks('sh').join('\n').match(/go get github\.com\/jackc\/pgx\/v5@(\S+)/)?.[1];
  assert.equal(shown, 'latest', 'the go get block on the page must take pgx@latest');
  // The examples build with Go 1.24: the prose names the version go.mod pins, and why.
  assert.ok(html.includes(`go get github.com/jackc/pgx/v5@${required}`), `the page must name pgx ${required}, the version go.mod requires`);
  assert.match(html, /GHSA-j88v-2chj-qfwx/);
});

test('week 8 shows the migration lock, parameters and the error mapping', () => {
  const go = blocks('go').join('\n');
  assert.match(go, /select pg_advisory_lock\(\$1\)/);
  assert.match(go, /pgx\.BeginFunc\(ctx, conn/);
  assert.match(go, /errors\.As\(err, &pgErr\) && pgErr\.Code == uniqueViolation/);
  assert.match(go, /where monitor_id = \$1/);
  assert.match(go, /"GET \/api\/monitors\/\{id\}\/results": h\.listResults/);
  // Every value reaches Postgres as a parameter: no Go code on the page glues SQL together.
  assert.doesNotMatch(go, /(?:select|insert|update|delete|where)[^"`\n]*["`]\s*\+/i);
});

test('CI runs the example tests against a Postgres 17 service', () => {
  const workflow = readFileSync(join(root, '.github/workflows/site.yml'), 'utf8');
  assert.match(workflow, /services:\n\s+postgres:\n\s+image: postgres:17\n/);
  assert.match(workflow, /DATABASE_URL: postgres:\/\/\S+@localhost:5432\/\S+/);
});

test('week 8 links to weeks 6 and 7 under the base path', () => {
  for (const slug of ['06-architecture-repo', '07-backend-api']) {
    assert.ok(html.includes(`href="${base}weeks/${slug}/"`), `no link to ${slug}`);
  }
});
