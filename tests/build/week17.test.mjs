import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { base, root } from './helpers.mjs';

// Week 17 scales the monitor, rolls it out and back, limits each client in the api and
// at Traefik, and backs up Postgres with a timed restore drill. These checks keep the
// page complete, and keep the limiter code and the manifests on the page in step with
// the tested example tree.

const html = readFileSync(join(root, 'dist/weeks/17-day-2-traffic/index.html'), 'utf8');
const figures = [...html.matchAll(/<figure class="dg"[^>]*>[\s\S]*?<\/figure>/g)].map((m) => m[0]);
const named = { amp: '&', lt: '<', gt: '>', quot: '"', apos: "'" };
const unescape = (s) =>
  s.replace(/&(?:#x([0-9a-f]+)|#(\d+)|(\w+));/gi, (m, hex, dec, name) =>
    hex ? String.fromCodePoint(parseInt(hex, 16)) : dec ? String.fromCodePoint(Number(dec)) : (named[name] ?? m));
const blocks = (lang) =>
  [...html.matchAll(new RegExp(`<pre data-language="${lang}">([\\s\\S]*?)</pre>`, 'g'))].map((m) =>
    unescape(m[1].replace(/<div class="ec-line[^"]*">/g, '\n').replace(/<[^>]+>/g, '')).replace(/^\n/, ''),
  );
const text = unescape(html.replace(/<[^>]+>/g, ' '));
const dir = join(root, 'examples/go/week17/your-monitor');

test('week 17 has three inline-SVG diagrams with walkthrough steps', () => {
  for (const f of figures) assert.match(f, /<svg[\s>]/, 'figure without inline <svg>');
  for (const id of ['hpa-loop', 'rolling-update', 'client-address']) {
    assert.ok(figures.some((f) => f.includes(`id="${id}"`) && f.includes('dg-steps')), `diagram ${id} has no walkthrough`);
  }
});

test('week 17 shows the client address walk, the bucket and the spoofing test', () => {
  const go = blocks('go').join('\n');
  assert.match(go, /func ClientIP\(r \*http\.Request, trusted \[\]netip\.Prefix\) netip\.Addr/);
  assert.match(go, /for i := len\(hops\) - 1; i >= 0; i--/, 'the walk must run from the right');
  assert.match(go, /ReserveN\(now, 1\)/);
  assert.match(go, /r\.CancelAt\(now\)/);
  assert.match(go, /func TestSpoofedHeaderDoesNotEscapeTheLimit/);
  assert.match(go, /"client spoofs behind a proxy that appends"/);
  assert.match(go, /w\.Header\(\)\.Set\("Retry-After"/);
});

test('week 17 tells the reader to go get golang.org/x/time at @latest', () => {
  const sh = blocks('sh').join('\n');
  assert.match(sh, /go get golang\.org\/x\/time@latest/);
  assert.doesNotMatch(sh, /x\/time@v\d/, 'the reader must not pin the examples version');
});

test('week 17 shows the HPA, the rollout settings, the Middleware and both CronJobs', () => {
  const yaml = blocks('yaml').join('\n');
  assert.match(yaml, /apiVersion: autoscaling\/v2\s+kind: HorizontalPodAutoscaler/);
  assert.match(yaml, /maxUnavailable: 0/);
  assert.match(yaml, /type: Recreate/);
  assert.match(yaml, /preStop:\s+sleep:\s+seconds: 5/);
  assert.match(yaml, /apiVersion: traefik\.io\/v1alpha1\s+kind: Middleware/);
  assert.match(yaml, /router\.middlewares: your-monitor-redirect-https@kubernetescrd,your-monitor-ratelimit@kubernetescrd/);
  assert.match(yaml, /resources: \[horizontalpodautoscalers\]/, 'the deployer Role must grant the HPAs');
  assert.match(yaml, /resources: \[cronjobs\]/, 'the deployer Role must grant the CronJobs');
  assert.match(yaml, /resources: \[persistentvolumeclaims\]/, 'the deployer Role must grant the backup claim');
  assert.match(yaml, /externalTrafficPolicy: Local/);
  assert.match(yaml, /kind: CronJob\s+metadata:\s+name: pg-backup/);
  assert.match(blocks('sh').join('\n'), /pg_restore --host="\$work"/, 'the drill script is not on the page');
});

test('week 17 validates against Kubernetes 1.36', () => {
  assert.match(blocks('sh').join('\n'), /-kubernetes-version 1\.36\.0/);
  assert.doesNotMatch(text, /1\.33/);
});

test('the limits on the page match the example manifests and code', () => {
  const api = readFileSync(join(dir, 'deploy/k8s/overlays/prod/api.yaml'), 'utf8');
  assert.match(api, /name: TRUSTED_PROXIES\s+value: 10\.42\.0\.0\/16/);
  assert.match(text, /10\.42\.0\.0\/16/);
  const limits = readFileSync(join(dir, 'internal/api/ratelimit.go'), 'utf8');
  assert.match(limits, /loginAllBurst = 3/);
  assert.match(limits, /loginAllRate {2}= 1/);
  assert.match(text, /one login a second after a burst of three/);
  const commands = `${text}\n${blocks('sh').join('\n')}\n${blocks('txt').join('\n')}`;
  for (const cmd of ['kubectl rollout status', 'kubectl rollout history', 'kubectl rollout undo', 'kubectl create job --from=cronjob/restore-drill']) {
    assert.ok(commands.includes(cmd), `${cmd} is not on the page`);
  }
});

test('week 17 links to weeks 8, 11, 13, 14 and 16 with the site base', () => {
  const hrefs = new Set([...html.matchAll(/href="([^"]+)"/g)].map((m) => m[1]));
  for (const slug of ['08-backend-postgres', '11-security-auth', '13-observability', '14-ci-cd', '16-going-live']) {
    const href = `${base}weeks/${slug}/`;
    assert.ok(hrefs.has(href), `no link to ${href}`);
    assert.ok(existsSync(join(root, 'dist/weeks', slug, 'index.html')), `${href} has no page`);
  }
});

test('week 17 has every standard box, the stage 3 note and no stub text', () => {
  for (const marker of ['callout callout-observe', 'callout callout-operate', 'callout callout-ai', 'side-lab', 'done-when']) {
    assert.match(html, new RegExp(`class="${marker}[ "]`), `missing ${marker}`);
  }
  assert.match(text, /Stage 3, pair partner/);
  assert.doesNotMatch(html, /not written yet|coming soon/i);
});

test('week 17 has four to six done items with kebab-case ids', () => {
  const ids = [...html.matchAll(/type="checkbox" value="([^"]+)"/g)].map((m) => m[1]);
  assert.ok(ids.length >= 4 && ids.length <= 6, `expected 4-6 done items, got ${ids.length}`);
  for (const id of ids) {
    assert.match(id, /^[a-z0-9]+(?:-[a-z0-9]+)*$/, `done id ${id} is not kebab-case`);
    assert.doesNotMatch(id, /^item-\d+$/, `done id ${id} is still a placeholder`);
  }
});
