import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { base, root } from './helpers.mjs';

// Week 15 runs the monitor on kind. These checks keep the page complete, keep the
// manifests on the page in step with the tested files, keep the kind node image and
// the kind release on the page in step with kind-config.yaml, and keep kubectl in CI
// for the manifest test.

const html = readFileSync(join(root, 'dist/weeks/15-kubernetes-local/index.html'), 'utf8');
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
const tree = 'examples/go/week15/your-monitor';
const file = (p) => readFileSync(join(root, tree, p), 'utf8');

test('week 15 has two inline-SVG diagrams, each with walkthrough steps', () => {
  assert.ok(figures.length >= 2, `expected >= 2 figure.dg, got ${figures.length}`);
  for (const f of figures) assert.match(f, /<svg[\s>]/, 'figure without inline <svg>');
  for (const id of ['kind-cluster', 'request-path']) {
    assert.ok(figures.some((f) => f.includes(`id="${id}"`) && f.includes('dg-steps')), `diagram ${id} has no walkthrough`);
  }
});

test('week 15 has every standard box and no stub text', () => {
  for (const marker of ['week-header', 'callout callout-observe', 'callout callout-operate', 'callout callout-ai', 'side-lab', 'done-when', 'read-with']) {
    assert.match(html, new RegExp(`class="${marker}[ "]`), `missing ${marker}`);
  }
  assert.doesNotMatch(html, /not written yet/i);
});

test('week 15 has four to six done items with kebab-case ids', () => {
  const ids = [...html.matchAll(/type="checkbox" value="([^"]+)"/g)].map((m) => m[1]);
  assert.ok(ids.length >= 4 && ids.length <= 6, `expected 4-6 done items, got ${ids.length}`);
  for (const id of ids) assert.match(id, /^[a-z0-9]+(?:-[a-z0-9]+)*$/, `done id ${id} is not kebab-case`);
});

test('week 15 shows the api probes and resources exactly as the tested api.yaml holds them', () => {
  const yaml = blocks('yaml').join('\n');
  for (const want of [/readinessProbe:\s+httpGet:\s+path: \/healthz\s+port: http/, /livenessProbe:\s+httpGet:\s+path: \/healthz/, /requests:\s+cpu: 50m\s+memory: 64Mi\s+limits:\s+cpu: 500m\s+memory: 256Mi/, /readOnlyRootFilesystem: true/, /volumeClaimTemplates:/, /secretGenerator:/]) {
    assert.match(yaml, want);
  }
  const api = file('deploy/k8s/base/api.yaml');
  assert.match(api, /path: \/healthz/);
  assert.ok(yaml.includes('image: your-monitor-api'), 'the page must name the image as Compose builds it');
});

test('the kind node image on the page is the one kind-config.yaml pins, from the kind release the page installs', () => {
  const image = /image: (kindest\/node:v1\.\d+\.\d+@sha256:[0-9a-f]{64})/.exec(file('deploy/k8s/kind-config.yaml'))?.[1];
  assert.ok(image, 'kind-config.yaml must pin a kindest/node image by digest');
  assert.ok(blocks('yaml').some((b) => b.includes(image)), 'the page must show the pinned node image');
  assert.ok(blocks('sh').some((b) => b.includes('go install sigs.k8s.io/kind@v0.33.0')), 'the page must install kind v0.33.0, whose release notes list that image');
});

// Kubernetes 1.36 for Weeks 15 to 18: the node image, the Pod Security
// labels, the kubeconform schemas, the kubectl of CI and of deploy.yml, and
// the kubectl the page installs must all name the same minor version.
test('every Kubernetes version names the same minor, 1.36', () => {
  const minors = {
    'kind-config.yaml node image': /kindest\/node:v1\.(\d+)\./.exec(file('deploy/k8s/kind-config.yaml'))?.[1],
    'namespace.yaml enforce-version': /enforce-version: v1\.(\d+)/.exec(file('deploy/k8s/overlays/kind/namespace.yaml'))?.[1],
    'namespace.yaml warn-version': /warn-version: v1\.(\d+)/.exec(file('deploy/k8s/overlays/kind/namespace.yaml'))?.[1],
    'Makefile kubeconform': /-kubernetes-version 1\.(\d+)\.0/.exec(file('Makefile'))?.[1],
    'deploy.yml kubectl': /version: v1\.(\d+)\.\d+/.exec(file('.github/workflows/deploy.yml'))?.[1],
    'site.yml kubectl': /dl\.k8s\.io\/release\/v1\.(\d+)\.\d+\/bin\/linux\/amd64\/kubectl/.exec(readFileSync(join(root, '.github/workflows/site.yml'), 'utf8'))?.[1],
    'page kubectl install': blocks('sh').join('\n').match(/stable-1\.(\d+)\.txt/)?.[1],
    'page kubeconform output': blocks('txt').join('\n').match(/-kubernetes-version 1\.(\d+)\.0/)?.[1],
    'page kubeconform schema URL': blocks('txt').join('\n').match(/\/v1\.(\d+)\.0-standalone/)?.[1],
    'page Pod Security warning': /restricted:v1\.(\d+)/.exec(html)?.[1],
  };
  for (const [where, minor] of Object.entries(minors)) {
    assert.equal(minor, '36', `${where} names Kubernetes 1.${minor}, want 1.36`);
  }
});

test('week 15 marks every kubectl and kind output as an example', () => {
  assert.match(html, /Every <code[^>]*>kubectl<\/code> and <code[^>]*>kind<\/code> output on this page is an example/);
});

test('CI installs kubectl for the manifest test', () => {
  const workflow = readFileSync(join(root, '.github/workflows/site.yml'), 'utf8');
  assert.match(workflow, /dl\.k8s\.io\/release\/v1\.\d+\.\d+\/bin\/linux\/amd64\/kubectl/);
  assert.match(workflow, /sha256sum --check/);
});

test('week 15 links to weeks 14 and 16 under the base path', () => {
  for (const slug of ['14-ci-cd', '16-going-live']) {
    assert.ok(html.includes(`href="${base}weeks/${slug}/"`), `no link to ${slug}`);
  }
});

// Three statements the review corrected: deleting the overlay deletes the
// data, restricted does not demand a read-only root filesystem, and a
// container without probes is never probed.
test('week 15 keeps the corrected statements on delete -k, the restricted standard and probes', () => {
  const text = html.replace(/<[^>]+>/g, '');
  assert.ok(text.includes('kubectl delete -k deploy/k8s/base'), 'the page must give delete -k on the base to keep the data');
  assert.match(text, /kubectl delete -k deploy\/k8s\/overlays\/kind deletes the data/);
  assert.doesNotMatch(text, /demands exactly these/);
  assert.match(text, /The restricted Pod Security Standard demands the first four/);
  assert.match(text, /A container without probes is never probed/);
});
