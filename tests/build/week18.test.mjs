import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { root } from './helpers.mjs';

// Week 18 is the last week: SLOs, burn-rate alerts, Alertmanager, an incident
// drill and a postmortem. These checks keep the page complete and keep the
// rules, tests and routes it shows the ones that promtool and amtool check.

const html = readFileSync(join(root, 'dist/weeks/18-sre/index.html'), 'utf8');
const figures = [...html.matchAll(/<figure class="dg"[^>]*>[\s\S]*?<\/figure>/g)].map((m) => m[0]);
// Expressive Code splits tokens into spans, so compare code on its text with tags removed.
const blocks = (lang) =>
  [...html.matchAll(new RegExp(`<pre data-language="${lang}">([\\s\\S]*?)<\\/pre>`, 'g'))].map((m) =>
    m[1].replace(/<[^>]+>/g, '').replace(/&gt;/g, '>').replace(/&lt;/g, '<').replace(/&quot;/g, '"').replace(/&#39;/g, "'").replace(/&amp;/g, '&'),
  );
const yaml = blocks('yaml').join('\n');

test('week 18 has at least two inline-SVG diagrams, both with walkthrough steps', () => {
  assert.ok(figures.length >= 2, `expected >= 2 figure.dg, got ${figures.length}`);
  for (const f of figures) assert.match(f, /<svg[\s>]/, 'figure without inline <svg>');
  const walkthroughs = figures.filter((f) => /<ol class="dg-steps"/.test(f));
  assert.ok(walkthroughs.length >= 2, `expected >= 2 diagrams with steps, got ${walkthroughs.length}`);
});

test('week 18 shows the burn-rate alerts with the workbook numbers', () => {
  for (const expr of [
    'slo:burn_rate:1h > 14.4 and slo:burn_rate:5m > 14.4',
    'slo:burn_rate:6h > 6 and slo:burn_rate:30m > 6',
    'slo:burn_rate:3d > 1 and slo:burn_rate:6h > 1',
  ]) {
    assert.ok(yaml.includes(expr), `the page does not show the alert expression ${expr}`);
  }
  assert.match(yaml, /record: slo:sli_error:ratio_rate5m/);
  assert.match(yaml, /vector\(0\.005\)/);
});

test('week 18 shows a promtool test that fires the fast burn and one that stays silent', () => {
  assert.match(yaml, /name: a full outage pages within five minutes[\s\S]*?alertname: ErrorBudgetFastBurn[\s\S]*?exp_labels:/);
  assert.match(yaml, /name: a two-minute blip does not page[\s\S]*?alertname: ErrorBudgetFastBurn\s+exp_alerts: \[\]/);
});

test('week 18 shows the Alertmanager routes, the postmortem template and the drill', () => {
  assert.match(yaml, /matchers: \['severity = "page"'\]/);
  assert.match(yaml, /telegram_configs:/);
  const md = blocks('md').join('\n');
  for (const section of ['## Timeline', '## Causes', '## Actions']) assert.ok(md.includes(section), `template without ${section}`);
  const sh = blocks('sh').join('\n');
  assert.match(sh, /kubectl -n your-monitor scale statefulset postgres --replicas=0/);
  assert.match(sh, /kubectl -n your-monitor scale statefulset postgres --replicas=1/);
});

test('week 18 has every standard box and no stub text', () => {
  for (const marker of ['callout callout-observe', 'callout callout-operate', 'callout callout-ai', 'side-lab', 'done-when']) {
    assert.match(html, new RegExp(`class="${marker}[ "]`), `missing ${marker}`);
  }
  assert.doesNotMatch(html, /coming soon|not written yet|DRILL_TIMELINE/i);
});

test('week 18 ends with a look back over the course and pointers onward', () => {
  const ids = [...html.matchAll(/<h2 id="([^"]+)"/g)].map((m) => m[1]);
  assert.deepEqual(ids.slice(-3), ['read-with-this-week', 'looking-back', 'where-to-go-next']);
  for (const id of ['observe-it', 'operate-it', 'ai-this-week', 'done-when']) assert.ok(ids.includes(id), `missing heading id ${id}`);
});

test('week 18 has four to six done items with kebab-case ids', () => {
  const ids = [...html.matchAll(/type="checkbox" value="([^"]+)"/g)].map((m) => m[1]);
  assert.ok(ids.length >= 4 && ids.length <= 6, `expected 4-6 done items, got ${ids.length}`);
  for (const id of ids) {
    assert.match(id, /^[a-z0-9]+(?:-[a-z0-9]+)*$/, `done id ${id} is not kebab-case`);
    assert.doesNotMatch(id, /^item-\d+$/, `done id ${id} is still a placeholder`);
  }
});
