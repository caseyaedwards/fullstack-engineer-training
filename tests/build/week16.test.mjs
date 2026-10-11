import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { base, root } from './helpers.mjs';

// Week 16 puts the monitor on a VPS. These checks keep the page complete and keep
// the doors it describes shut: the API server stays behind the SSH tunnel, CI gets a
// namespaced Role without delete or Secrets, and the reader picks the cert-manager
// release instead of copying a version from the page.

const html = readFileSync(join(root, 'dist/weeks/16-going-live/index.html'), 'utf8');
const figures = [...html.matchAll(/<figure class="dg"[^>]*>[\s\S]*?<\/figure>/g)].map((m) => m[0]);
// Expressive Code splits tokens into spans and lines into divs: compare code on its text.
const named = { amp: '&', lt: '<', gt: '>', quot: '"', apos: "'" };
const unescape = (s) =>
  s.replace(/&(?:#x([0-9a-f]+)|#(\d+)|(\w+));/gi, (m, hex, dec, name) =>
    hex ? String.fromCodePoint(parseInt(hex, 16)) : dec ? String.fromCodePoint(Number(dec)) : (named[name] ?? m));
// Comments explain what a file leaves out: check the lines that take effect.
const code = (s) => s.split('\n').filter((l) => !/^\s*#/.test(l)).join('\n');
const blocks = (lang) =>
  [...html.matchAll(new RegExp(`<pre data-language="${lang}">([\\s\\S]*?)</pre>`, 'g'))].map((m) =>
    unescape(m[1].replace(/<div class="ec-line[^"]*">/g, '\n').replace(/<[^>]+>/g, '')).replace(/^\n/, ''),
  );

test('week 16 has three inline-SVG diagrams, each with walkthrough steps', () => {
  assert.ok(figures.length >= 3, `expected >= 3 figure.dg, got ${figures.length}`);
  for (const f of figures) assert.match(f, /<svg[\s>]/, 'figure without inline <svg>');
  for (const id of ['request-path', 'http01', 'deploy-path']) {
    assert.ok(figures.some((f) => f.includes(`id="${id}"`) && f.includes('dg-steps')), `diagram ${id} has no walkthrough`);
  }
});

test('week 16 has every standard box and no stub text', () => {
  for (const marker of ['week-header', 'callout callout-observe', 'callout callout-operate', 'callout callout-ai', 'side-lab', 'done-when', 'read-with']) {
    assert.match(html, new RegExp(`class="${marker}[ "]`), `missing ${marker}`);
  }
  assert.doesNotMatch(html, /not written yet/i);
});

test('week 16 has four to six done items with kebab-case ids', () => {
  const ids = [...html.matchAll(/type="checkbox" value="([^"]+)"/g)].map((m) => m[1]);
  assert.ok(ids.length >= 4 && ids.length <= 6, `expected 4-6 done items, got ${ids.length}`);
  for (const id of ids) assert.match(id, /^[a-z0-9]+(?:-[a-z0-9]+)*$/, `done id ${id} is not kebab-case`);
});

test('week 16 gives CI a namespaced Role without delete, Secrets or RBAC', () => {
  const yaml = blocks('yaml').join('\n---\n');
  const role = blocks('yaml').find((b) => /^kind: Role$/m.test(b));
  assert.ok(role, 'no YAML block with kind: Role');
  assert.match(role, /namespace: your-monitor/);
  assert.match(role, /verbs: \[get, create, patch\]/);
  const rules = code(role.slice(role.indexOf('rules:')));
  for (const word of ['delete', 'secrets', 'pods', '*', 'rbac.authorization.k8s.io', 'escalate', 'bind', 'impersonate']) {
    assert.ok(!rules.includes(word), `the Role grants ${word}`);
  }
  assert.doesNotMatch(yaml, /^kind: Cluster(Role|RoleBinding)$/m, 'the page shows a ClusterRole or ClusterRoleBinding');
  // The kubeconfig carries the namespace: deploy.yml runs kubectl without -n.
  assert.match(blocks('sh').join('\n'), /config set-context deploy --cluster=vps --user=deployer --namespace=your-monitor/);
});

test('week 16 keeps port 6443 behind the SSH tunnel', () => {
  const conf = blocks('txt').join('\n');
  assert.match(conf, /Match User tunnel[\s\S]*PermitOpen 127\.0\.0\.1:6443/);
  assert.match(conf, /ForceCommand \/usr\/sbin\/nologin/);
  assert.match(conf, /PasswordAuthentication no/);
  const yaml = blocks('yaml').join('\n');
  assert.match(yaml, /-L 127\.0\.0\.1:6443:127\.0\.0\.1:6443/);
  assert.match(yaml, /StrictHostKeyChecking=yes/);
  const firewall = blocks('sh').find((b) => b.includes('ufw default deny incoming'));
  assert.ok(firewall, 'no block shows firewall.sh');
  for (const port of ['22', '80', '443']) assert.match(firewall, new RegExp(`ufw allow ${port}/tcp`));
  assert.doesNotMatch(code(firewall), /6443/);
});

test('week 16 closes the ports that ufw cannot see and enforces restricted Pod Security', () => {
  const yaml = blocks('yaml').join('\n');
  assert.match(yaml, /kind: ValidatingAdmissionPolicy\n/);
  assert.match(yaml, /expression: object\.spec\.type == 'ClusterIP'/);
  assert.match(yaml, /validationActions: \[Deny\]/);
  for (const label of ['enforce: restricted', 'enforce-version: v1.36', 'warn: restricted', 'warn-version: v1.36']) {
    assert.ok(yaml.includes(`pod-security.kubernetes.io/${label}`), `the namespace lacks ${label}`);
  }
  assert.doesNotMatch(html, /baseline:latest/);
});

test('week 16 runs Kubernetes 1.36 on the VPS and checks the overlay against it', () => {
  const sh = blocks('sh').join('\n');
  assert.match(sh, /INSTALL_K3S_CHANNEL=v1\.36 sh k3s-install\.sh/);
  assert.match(sh, /-kubernetes-version 1\.36\.0/);
  assert.doesNotMatch(html, /Kubernetes 1\.33|v1\.33|1\.33\.0/);
});

test('week 16 shows the RBAC test where the reader keeps it, with its render helper', () => {
  const go = blocks('go').join('\n');
  assert.match(go, /func render\(t \*testing\.T, dir string\) \(objs \[\]object, rendered bool\)/);
  assert.match(go, /exec\.Command\(kubectl, "kustomize", filepath\.Join\(tmp, dir\)\)/);
  assert.match(go, /func TestDeployerCanApplyTheProdOverlay/);
  assert.match(go, /func allows\(rules \[\]rule/);
  assert.ok(html.includes('deploy/k8s/manifests_test.go'), 'the page must say where the test goes');
});

test('week 16 issues from Let\'s Encrypt staging first and reads the cert-manager release', () => {
  const yaml = blocks('yaml').join('\n');
  assert.ok(yaml.includes('https://acme-staging-v02.api.letsencrypt.org/directory'), 'no staging server');
  assert.ok(yaml.includes('https://acme-v02.api.letsencrypt.org/directory'), 'no production server');
  assert.match(yaml, /ingressClassName: traefik/);
  const sh = blocks('sh').join('\n');
  assert.match(sh, /releases\/latest/);
  assert.ok(sh.includes('releases/download/$CM_VERSION/cert-manager.yaml'), 'the install must use $CM_VERSION');
  assert.doesNotMatch(html, /cert-manager\/releases\/download\/v\d/, 'the page hard-codes a cert-manager version');
});

test('week 16 links to weeks 14 and 15 under the base path', () => {
  for (const slug of ['14-ci-cd', '15-kubernetes-local']) {
    assert.ok(html.includes(`href="${base}weeks/${slug}/"`), `no link to ${slug}`);
  }
});
