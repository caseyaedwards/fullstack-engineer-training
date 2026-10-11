// Code blocks keep tab characters. make needs a tab before each recipe line, so a
// Makefile copied from the page with spaces fails with "missing separator".
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { root } from './helpers.mjs';

const page = readFileSync(join(root, 'dist/weeks/06-architecture-repo/index.html'), 'utf8');

test('the Week 6 Makefile keeps its tabs in the copy button text', () => {
  const copies = [...page.matchAll(/data-code="([^"]*)"/g)].map((m) => m[1]);
  const makefile = copies.find((c) => c.includes('.PHONY'));
  assert.ok(makefile, 'no code block with .PHONY on the Week 6 page');
  assert.match(makefile, /\n\t|\x7f\t|&#x9;|\t/, 'the Makefile copy text has no tab');
  assert.doesNotMatch(makefile, /\x7f  [a-z]/, 'a recipe line starts with two spaces instead of a tab');
});
