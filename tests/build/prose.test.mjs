import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { phases, weeks } from '../../src/data/weeks.ts';
import { trackGroups, tracks } from '../../src/data/tracks.ts';
import { checkHeading, formatProblem, lintSource } from '../lib/prose.mjs';
import { docsDir, mdxFiles } from './helpers.mjs';

// Enforces docs/style-guide.md on the site copy: banned and hedging words, contractions,
// e.g. and i.e., "here" link text, and Title Case headings without backticks.
// It reads the sources, so it does not need dist/.

/** Program and code names that stay lowercase in a heading. Add one here when a heading needs it. */
const CODE_WORDS = new Set([
  'bash', 'curl', 'dig', 'docker', 'git', 'go', 'grep', 'httptrace', 'jq', 'kubectl', 'npm', 'ps', 'ssh', 'strace', 'tcpdump',
]);

// dev/ holds the diagram fixture page, which is test data and not part of the course.
const files = mdxFiles(docsDir).filter((f) => !f.startsWith('dev/'));

test('the prose lint covers every page except dev/', () => {
  assert.ok(files.includes('index.mdx') && files.includes('start-here.mdx'));
  assert.equal(files.filter((f) => f.startsWith('weeks/')).length, weeks.length);
  assert.equal(files.filter((f) => f.startsWith('tracks/')).length, tracks.length);
});

for (const f of files) {
  test(`prose: ${f} follows docs/style-guide.md`, () => {
    const problems = lintSource(readFileSync(join(docsDir, f), 'utf8'), { codeWords: CODE_WORDS });
    assert.deepEqual(problems.map((p) => formatProblem(`src/content/docs/${f}`, p)), []);
  });
}

test('registry titles shown as headings and sidebar labels are Title Case', () => {
  const labels = [
    ...weeks.map((w) => ['src/data/weeks.ts', w.title]),
    ...phases.map((p) => ['src/data/weeks.ts', p.label]),
    ...tracks.map((t) => ['src/data/tracks.ts', t.title]),
    ...trackGroups.map((g) => ['src/data/tracks.ts', g.label]),
  ];
  const problems = labels.flatMap(([file, label]) =>
    checkHeading(label, CODE_WORDS).map((p) => `${file}: "${label}": ${p.rule} "${p.word}" (${p.fix})`),
  );
  assert.deepEqual(problems, []);
});
