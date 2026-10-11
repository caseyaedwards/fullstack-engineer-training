import { test } from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, utimesSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { base, root } from './helpers.mjs';

// Week 6 splits the monitor into a monorepo and starts it with Docker Compose. These
// checks keep the page complete, keep the Dockerfile, compose.yaml and Makefile on the
// page in step with the tested example files, and check the shape of the Compose file.

const html = readFileSync(join(root, 'dist/weeks/06-architecture-repo/index.html'), 'utf8');
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
const dir = join(root, 'examples/go/week06/your-monitor');
const example = (file) => readFileSync(join(dir, file), 'utf8');

test('week 6 has at least three inline-SVG diagrams, two with walkthrough steps', () => {
  assert.ok(figures.length >= 3, `expected >= 3 figure.dg, got ${figures.length}`);
  for (const f of figures) assert.match(f, /<svg[\s>]/, 'figure without inline <svg>');
  const walkthroughs = figures.filter((f) => /<ol class="dg-steps"/.test(f));
  assert.ok(walkthroughs.length >= 2, `expected >= 2 diagrams with steps, got ${walkthroughs.length}`);
  for (const id of ['target-architecture', 'dev-proxy']) {
    assert.ok(figures.some((f) => f.includes(`id="${id}"`) && f.includes('dg-steps')), `diagram ${id} has no walkthrough`);
  }
});

test('week 6 shows a multi-stage Dockerfile with a static, non-root final image', () => {
  const docker = blocks('dockerfile').join('\n');
  assert.match(docker, /FROM golang:\$\{GO_VERSION\}-alpine AS build/);
  assert.match(docker, /RUN CGO_ENABLED=0 go build /);
  assert.match(docker, /FROM scratch AS runtime/);
  assert.match(docker, /COPY --from=build \/etc\/ssl\/certs\/ca-certificates\.crt/);
  assert.match(docker, /USER 65532:65532/);
  assert.match(docker, /ENTRYPOINT \["\/api"\]/);
  assert.match(docker, /ENTRYPOINT \["\/worker"\]/);
});

test('week 6 shows compose.yaml with Postgres 17, a named volume and health-gated start order', () => {
  const yaml = blocks('yaml').join('\n');
  assert.match(yaml, /image: postgres:17\n/);
  assert.match(yaml, /- pgdata:\/var\/lib\/postgresql\/data/);
  assert.equal((yaml.match(/condition: service_healthy/g) ?? []).length, 2, 'api and worker must both wait for postgres');
  assert.match(yaml, /"127\.0\.0\.1:\$\{API_PORT:-8080\}:8080"/);
});

test('week 6 shows the api stub with its /healthz route and tests', () => {
  const go = blocks('go').join('\n');
  assert.match(go, /mux\.HandleFunc\("GET \/healthz", healthz\)/);
  assert.match(go, /flag\.String\("healthcheck"/);
  assert.match(go, /func TestHealthz\(t \*testing\.T\)/);
  assert.match(go, /func TestServeStopsWhenCancelled\(t \*testing\.T\)/);
});

test('the .env.example and .dockerignore on the page match the example files', () => {
  const txt = blocks('txt');
  for (const file of ['.env.example', '.dockerignore']) {
    const want = example(file).trimEnd();
    assert.ok(txt.some((b) => b.trimEnd() === want), `no txt block on the page equals ${file}:\n${want}`);
  }
});

test('the Compose healthcheck runs the binary the Dockerfile installs, with a command-line flag main.go defines', () => {
  const compose = example('compose.yaml');
  const m = /test: \["CMD", "(\/\w+)", "-(\w+)", "http:\/\/127\.0\.0\.1:(\d+)\/healthz"\]/.exec(compose);
  assert.ok(m, 'no api healthcheck of the form ["CMD", "/api", "-healthcheck", "http://127.0.0.1:PORT/healthz"]');
  const [, bin, flag, port] = m;
  assert.match(example('Dockerfile'), new RegExp(`COPY --from=build /out/api ${bin}\\n`));
  assert.match(example('cmd/api/main.go'), new RegExp(`flag\\.String\\("${flag}"`));
  assert.match(example('cmd/api/main.go'), new RegExp(`flag\\.String\\("addr", ":${port}"`));
  assert.match(compose, new RegExp(`:${port}"\\n`), 'the published port must target the port the api listens on');
});

// docker compose config parses and checks the file without a Docker engine. CI has the
// Docker CLI with Compose; a machine without it skips this test, except in CI.
const compose = spawnSync('docker', ['compose', 'version'], { encoding: 'utf8' });
const haveCompose = compose.status === 0;
test('docker compose config accepts compose.yaml and gives it the expected shape', { skip: !haveCompose && !process.env.CI && 'docker compose is not installed' }, () => {
  assert.ok(haveCompose, 'docker compose must be installed in CI');
  const out = execFileSync('docker', ['compose', '--env-file', '.env.example', 'config', '--format', 'json'], { cwd: dir, encoding: 'utf8' });
  const { name, services, volumes } = JSON.parse(out);
  assert.equal(name, 'your-monitor');
  assert.deepEqual(Object.keys(services).sort(), ['api', 'postgres', 'worker']);
  assert.equal(services.postgres.image, 'postgres:17');
  assert.deepEqual(
    services.postgres.volumes.map((v) => [v.type, v.source, v.target]),
    [['volume', 'pgdata', '/var/lib/postgresql/data']],
  );
  assert.ok(volumes.pgdata, 'pgdata must be a named volume');
  for (const svc of ['api', 'worker']) {
    assert.equal(services[svc].build.target, svc, `${svc} must build its own Dockerfile target`);
    assert.equal(services[svc].depends_on?.postgres?.condition, 'service_healthy', `${svc} must wait for a healthy postgres`);
  }
  for (const svc of ['postgres', 'api']) {
    assert.equal(services[svc].healthcheck?.test?.[0], 'CMD', `${svc} needs a healthcheck`);
    for (const p of services[svc].ports) assert.equal(p.host_ip, '127.0.0.1', `${svc} must publish on 127.0.0.1 only`);
  }
  assert.ok(services.worker.volumes.some((v) => v.type === 'bind' && v.target === '/etc/your-monitor/urls.txt' && v.read_only));
  assert.ok(services.worker.command.includes('/etc/your-monitor/urls.txt'));
});

test('compose.yaml refuses to start without a Postgres password', { skip: !haveCompose && !process.env.CI && 'docker compose is not installed' }, () => {
  const env = { ...process.env };
  delete env.POSTGRES_PASSWORD;
  const r = spawnSync('docker', ['compose', '--env-file', '/dev/null', 'config'], { cwd: dir, encoding: 'utf8', env });
  assert.notEqual(r.status, 0);
  assert.match(r.stderr, /required variable POSTGRES_PASSWORD is missing a value: set POSTGRES_PASSWORD in \.env/);
});

test('the Makefile has build, test, run, up and down targets that run the commands the page describes', () => {
  // A copy with a stand-in web/ folder: make -n prints the recipes without running them.
  const tmp = mkdtempSync(join(tmpdir(), 'week06-make-'));
  try {
    cpSync(join(dir, 'Makefile'), join(tmp, 'Makefile'));
    mkdirSync(join(tmp, 'web/node_modules'), { recursive: true });
    writeFileSync(join(tmp, 'web/package-lock.json'), '{}');
    const make = (...targets) => execFileSync('make', ['-n', ...targets], { cwd: tmp, encoding: 'utf8' });

    // node_modules is older than package-lock.json: make test installs the packages first.
    utimesSync(join(tmp, 'web/node_modules'), new Date(0), new Date(0));
    assert.equal(
      make('test'),
      'npm --prefix web ci\ngo vet ./...\ngo test -race ./...\nnpm --prefix web test\n',
    );
    // node_modules is newer: the install is skipped.
    utimesSync(join(tmp, 'web/node_modules'), new Date(), new Date());
    assert.equal(make('test'), 'go vet ./...\ngo test -race ./...\nnpm --prefix web test\n');

    assert.equal(make('build'), 'CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/ ./cmd/...\n');
    assert.equal(make('run'), 'go run ./cmd/worker -file urls.txt -interval 30s\n');
    assert.equal(make('run', 'ARGS=-file urls.txt -workers 20'), 'go run ./cmd/worker -file urls.txt -workers 20\n');
    assert.equal(make('up'), 'docker compose up --build --detach\n');
    assert.equal(make('down'), 'docker compose down\n');
  } finally {
    rmSync(tmp, { recursive: true, force: true });
  }
});

test('week 6 links to weeks 5, 7 and 8 with the site base', () => {
  const hrefs = new Set([...html.matchAll(/href="([^"]+)"/g)].map((m) => m[1]));
  for (const slug of ['05-concurrency', '07-backend-api', '08-backend-postgres']) {
    const href = `${base}weeks/${slug}/`;
    assert.ok(hrefs.has(href), `no link to ${href}`);
    assert.ok(existsSync(join(root, 'dist/weeks', slug, 'index.html')), `${href} has no page`);
  }
});

test('week 6 has every standard box and no stub text', () => {
  for (const marker of ['callout callout-observe', 'callout callout-operate', 'callout callout-ai', 'side-lab', 'done-when']) {
    assert.match(html, new RegExp(`class="${marker}[ "]`), `missing ${marker}`);
  }
  assert.doesNotMatch(html, /not written yet|coming soon/i);
});

test('week 6 has four to six done items with kebab-case ids', () => {
  const ids = [...html.matchAll(/type="checkbox" value="([^"]+)"/g)].map((m) => m[1]);
  assert.ok(ids.length >= 4 && ids.length <= 6, `expected 4-6 done items, got ${ids.length}`);
  for (const id of ids) {
    assert.match(id, /^[a-z0-9]+(?:-[a-z0-9]+)*$/, `done id ${id} is not kebab-case`);
    assert.doesNotMatch(id, /^item-\d+$/, `done id ${id} is still a placeholder`);
  }
});
