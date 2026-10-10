import { test } from 'node:test';
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../../', import.meta.url));
const devDir = join(root, 'src/content/docs/dev');

test('the diagrams fixture page renders the timings region of trace.go', () => {
  const html = readFileSync(join(root, 'dist/dev/diagrams/index.html'), 'utf8');
  // Expressive Code splits tokens into spans, so match on the text with tags removed.
  const text = html.replace(/<[^>]+>/g, '');
  assert.match(text, /httptrace\.ClientTrace/);
  assert.doesNotMatch(text, /region: timings/, 'region markers must not be rendered');
  assert.doesNotMatch(text, /package week04/, 'only the region should be rendered, not the whole file');
});

/** Build with one fixture page using <CodeFrom> and return the combined output. */
function buildWith(name, props) {
  const fixture = join(devDir, `${name}.mdx`);
  const outDir = mkdtempSync(join(tmpdir(), 'codefrom-'));
  try {
    writeFileSync(
      fixture,
      `---
title: CodeFrom fixture
---
import CodeFrom from '../../../components/CodeFrom.astro';

<CodeFrom ${props} lang="go" />
`,
    );
    const r = spawnSync('npm', ['run', 'build', '--', '--outDir', outDir], { cwd: root, encoding: 'utf8', env: { ...process.env, NO_COLOR: '1' } });
    return { status: r.status, out: r.stdout + r.stderr };
  } finally {
    rmSync(fixture, { force: true });
    rmSync(outDir, { recursive: true, force: true });
  }
}

for (const [name, props, expected] of [
  ['zz-codefrom-region', 'file="go/week04/trace.go" region="missing"', [/region "missing" not found in examples\/go\/week04\/trace\.go/]],
  ['zz-codefrom-file', 'file="go/week04/nope.go" region="timings"', [/no file at examples\/go\/week04\/nope\.go/]],
  ['zz-codefrom-escape', 'file="../package.json" region="timings"', [/\.\.\/package\.json/, /stay inside the examples\/ directory/]],
]) {
  test(`${name}: a bad <CodeFrom> fails the build and names what is wrong`, { timeout: 300_000 }, () => {
    const r = buildWith(name, props);
    assert.notEqual(r.status, 0, 'build should fail');
    for (const re of expected) assert.match(r.out, re);
  });
}
