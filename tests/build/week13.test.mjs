import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import { base, root } from './helpers.mjs';

// Week 13 adds JSON logs, Prometheus metrics and OpenTelemetry traces to the monitor,
// and Prometheus, Grafana, the collector and Jaeger to Compose. These checks keep the
// page complete, and keep every metric name on the page in step with the tested code.

const html = readFileSync(join(root, 'dist/weeks/13-observability/index.html'), 'utf8');
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
const text = unescape(html.replace(/<[^>]+>/g, ' '));
const dir = join(root, 'examples/go/week13/your-monitor');

// Every non-test Go file of the example, joined.
function goSource() {
  const out = [];
  const walk = (d) => {
    for (const e of readdirSync(d, { withFileTypes: true })) {
      if (e.isDirectory()) walk(join(d, e.name));
      else if (e.name.endsWith('.go') && !e.name.endsWith('_test.go')) out.push(readFileSync(join(d, e.name), 'utf8'));
    }
  };
  walk(dir);
  return out.join('\n');
}

test('week 13 has at least two inline-SVG diagrams, signals and job-trace with walkthrough steps', () => {
  assert.ok(figures.length >= 2, `expected >= 2 figure.dg, got ${figures.length}`);
  for (const f of figures) assert.match(f, /<svg[\s>]/, 'figure without inline <svg>');
  for (const id of ['signals', 'job-trace']) {
    assert.ok(figures.some((f) => f.includes(`id="${id}"`) && f.includes('dg-steps')), `diagram ${id} has no walkthrough`);
  }
});

test('every metric name on the page is registered by the example code', () => {
  const src = goSource();
  const onPage = new Set([...text.matchAll(/\b((?:http|monitor)_[a-z_]*?(?:total|seconds|queued))(?:_bucket|_sum|_count)?\b/g)].map((m) => m[1]));
  for (const name of ['http_requests_total', 'http_request_duration_seconds', 'monitor_checks_total', 'monitor_check_duration_seconds', 'monitor_jobs_queued', 'monitor_jobs_oldest_seconds']) {
    assert.ok(onPage.has(name), `${name} is not on the page`);
  }
  for (const name of onPage) {
    assert.match(src, new RegExp(`Name:\\s+"${name}"`), `${name} is on the page but the example code registers no such metric`);
  }
  // The label sets the page names.
  assert.match(src, /\[\]string\{"method", "route", "code"\}/);
  assert.match(src, /\[\]string\{"result"\}/);
});

test('the worker histogram has bounds at the timeout and above it, and the logs use one spelling of each field', () => {
  const go = blocks('go').join('\n');
  assert.match(go, /Buckets: \[\]float64\{[^}]*\b5, 6\}/, 'the check histogram needs bounds at 5 s (-timeout) and 6 s');
  const txt = blocks('txt').join('\n');
  for (const line of txt.split('\n').filter((l) => l.startsWith('{"time"'))) {
    assert.doesNotMatch(line, /"(job|monitor|incident|error)":/, `log line uses an old field name: ${line}`);
  }
  assert.doesNotMatch(readFileSync(join(dir, 'observability/prometheus.yml'), 'utf8'), /dns_sd_configs/, 'the worker drill expects up{job="worker"} 0 from a static target');
});

test('week 13 tells the reader to go get the libraries at @latest', () => {
  const sh = blocks('sh').join('\n');
  assert.match(sh, /go get github\.com\/prometheus\/client_golang@latest/);
  assert.match(sh, /go get go\.opentelemetry\.io\/otel@latest go\.opentelemetry\.io\/otel\/sdk@latest go\.opentelemetry\.io\/otel\/exporters\/otlp\/otlptrace\/otlptracehttp@latest/);
  assert.doesNotMatch(sh, /client_golang@v|otel[^ ]*@v\d/, 'the reader must not pin the examples versions');
});

test('week 13 shows the logger, the middleware, the tracer and the configs', () => {
  const go = blocks('go').join('\n');
  assert.match(go, /func NewLogger\(w io\.Writer, service string\) \*slog\.Logger/);
  assert.match(go, /func \(m \*Metrics\) Instrument\(next http\.Handler\) http\.Handler/);
  assert.match(go, /func Trace\(next http\.Handler\) http\.Handler/);
  assert.match(go, /func StartTracing\(ctx context\.Context, service string\)/);
  assert.match(go, /func \(QueryTracer\) TraceQueryStart/);
  assert.match(go, /tracetest\.NewInMemoryExporter\(\)/);
  const yaml = blocks('yaml').join('\n');
  assert.match(yaml, /scrape_configs:/);
  assert.match(yaml, /otlp_grpc\/jaeger:/);
  assert.match(yaml, /prom\/prometheus:v\d+\.\d+\.\d+/);
});

test('week 13 links to weeks 0, 7, 11 and 12 with the site base', () => {
  const hrefs = new Set([...html.matchAll(/href="([^"]+)"/g)].map((m) => m[1]));
  for (const slug of ['00-setup', '07-backend-api', '11-security-auth', '12-debugging']) {
    const href = `${base}weeks/${slug}/`;
    assert.ok(hrefs.has(href), `no link to ${href}`);
    assert.ok(existsSync(join(root, 'dist/weeks', slug, 'index.html')), `${href} has no page`);
  }
});

test('week 13 has every standard box, the stage 3 note and no stub text', () => {
  for (const marker of ['callout callout-observe', 'callout callout-operate', 'callout callout-ai', 'side-lab', 'done-when']) {
    assert.match(html, new RegExp(`class="${marker}[ "]`), `missing ${marker}`);
  }
  assert.match(text, /Stage 3, pair partner, starts this week/);
  assert.doesNotMatch(html, /not written yet|coming soon/i);
});

test('week 13 has four to six done items with kebab-case ids', () => {
  const ids = [...html.matchAll(/type="checkbox" value="([^"]+)"/g)].map((m) => m[1]);
  assert.ok(ids.length >= 4 && ids.length <= 6, `expected 4-6 done items, got ${ids.length}`);
  for (const id of ids) {
    assert.match(id, /^[a-z0-9]+(?:-[a-z0-9]+)*$/, `done id ${id} is not kebab-case`);
    assert.doesNotMatch(id, /^item-\d+$/, `done id ${id} is still a placeholder`);
  }
});
