import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { base, root } from './helpers.mjs';

// Week 0 has no Go package: its "code" is the commands the reader runs. These checks keep
// the page complete: the diagrams, every version check, the commands that create the
// repository Week 1 builds on, the weekly Git rhythm, and the AI ladder.
// tests/build/weeks.test.mjs covers the standard boxes and done items of every written week.

const html = readFileSync(join(root, 'dist/weeks/00-setup/index.html'), 'utf8');
const figures = [...html.matchAll(/<figure class="dg"[^>]*>[\s\S]*?<\/figure>/g)].map((m) => m[0]);

const decode = (s) =>
  s
    .replace(/<[^>]+>/g, '')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&quot;/g, '"')
    .replace(/&#39;|&#x27;/g, "'")
    .replace(/&amp;/g, '&');
// Expressive Code splits tokens into spans and puts one line per div, so compare code
// blocks on their text, one line each.
const blocks = [...html.matchAll(/<pre data-language="([^"]+)">([\s\S]*?)<\/pre>/g)].map((m) => ({
  lang: m[1],
  text: m[2].replace(/<\/div>/g, '\n').replace(/<[^>]+>/g, '').split('\n').map(decode).join('\n'),
}));
const code = blocks.map((b) => b.text).join('\n');
const prose = decode(html);

test('week 0 has two inline-SVG diagrams with walkthrough steps', () => {
  assert.ok(figures.length >= 2, `expected >= 2 figure.dg, got ${figures.length}`);
  for (const id of ['toolchain', 'weekly-rhythm']) {
    const f = figures.find((x) => x.includes(`id="${id}"`));
    assert.ok(f, `missing diagram ${id}`);
    assert.match(f, /<svg[\s>]/, `${id}: no inline <svg>`);
    assert.match(f, /<ol class="dg-steps"/, `${id}: no walkthrough steps`);
  }
});

test('week 0 shows a version check for every tool in the toolchain', () => {
  for (const cmd of ['git --version', 'go version', 'node --version', 'gh --version', 'curl --version', 'docker --version', 'docker compose version', 'code --version', 'docker run hello-world']) {
    assert.ok(code.includes(cmd), `no code block shows ${cmd}`);
  }
});

test('week 0 creates the repository that Week 1 starts from', () => {
  assert.match(code, /git clone https:\/\/github\.com\/<user>\/your-monitor\.git/);
  assert.match(code, /go mod init github\.com\/<user>\/your-monitor/);
  assert.match(code, /git add go\.mod README\.md \.gitignore/);
  assert.match(code, /git push -u origin main/);
  const gitignore = blocks.find((b) => b.text.includes('/your-monitor') && b.text.includes('.env'));
  assert.ok(gitignore, 'no .gitignore block that ignores the built program and .env');
});

test('week 0 walks through the weekly rhythm: branch, commit, push, pull request, merge, pull', () => {
  for (const cmd of ['git switch -c week-00', 'git commit -m', 'git push -u origin week-00', 'git switch main', 'git pull', 'git branch -d week-00']) {
    assert.ok(code.includes(cmd), `no code block shows ${cmd}`);
  }
  for (const ui of ['Compare & pull request', 'Create pull request', 'Files changed', 'Merge pull request', 'Confirm merge']) {
    assert.ok(prose.includes(ui), `the page never names the GitHub button or tab ${ui}`);
  }
});

test('week 0 links Week 1 and the AI track from its text, with the site base', () => {
  const count = (path) => html.split(`href="${base}${path}"`).length - 1;
  // The sidebar and the Next pagination link Week 1 once each, and the sidebar and Read with
  // link each track once each; any further link is in the text.
  assert.ok(count('weeks/01-go-basics/') >= 3, 'no link to Week 1 in the page text');
  assert.ok(count('tracks/working/ai-assisted-dev/') >= 3, 'no link to the AI-Assisted Development track in the page text');
});

test('week 0 sets up what Week 1 relies on: format and organize imports on save, and a running Docker engine', () => {
  assert.match(prose, /format on save/);
  assert.match(prose, /organize imports on save/);
  assert.match(prose, /removes every import that your code does not use/);
  assert.ok(code.includes('code --install-extension golang.go'));
  assert.match(prose, /Hello from Docker!/);
});

test('week 0 states the AI ladder and points Windows readers to WSL 2', () => {
  for (const stage of ['Explain only', 'Review everything', 'Pair partner']) {
    assert.ok(prose.includes(stage), `the AI ladder is missing stage ${stage}`);
  }
  assert.match(prose, /WSL 2/);
  assert.match(prose, /editor\.inlineSuggest\.enabled/);
});
