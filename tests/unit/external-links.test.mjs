import assert from 'node:assert/strict';
import { test } from 'node:test';
import { extractLinks } from '../../scripts/check-external-links.mjs';

test('finds links in prose and in list items', () => {
  const source = [
    'Read [RFC 9110](https://www.rfc-editor.org/rfc/rfc9110) first, then',
    '[the Go tour](https://go.dev/tour/) too.',
    '',
    '- [Kubernetes](https://kubernetes.io/docs/concepts/)',
    '  * [SRE book](https://sre.google/sre-book/table-of-contents/#intro)',
  ].join('\n');
  assert.deepEqual(extractLinks(source), [
    'https://www.rfc-editor.org/rfc/rfc9110',
    'https://go.dev/tour/',
    'https://kubernetes.io/docs/concepts/',
    'https://sre.google/sre-book/table-of-contents/#intro',
  ]);
});

test('skips links inside fenced code blocks', () => {
  const source = [
    '```md',
    '[nope](https://example.com/fenced)',
    '```',
    '',
    '~~~',
    '[nope](https://example.com/tilde)',
    '~~~',
    '',
    '[yes](https://example.com/after)',
  ].join('\n');
  assert.deepEqual(extractLinks(source), ['https://example.com/after']);
});

test('a longer fence is not closed by a shorter one', () => {
  const source = [
    '````',
    '```',
    '[nope](https://example.com/inner)',
    '```',
    '````',
    '[yes](https://example.com/after)',
  ].join('\n');
  assert.deepEqual(extractLinks(source), ['https://example.com/after']);
});

test('skips links inside inline code', () => {
  const source =
    'Write `[x](https://example.com/inline)` like so, or ``[y](https://example.com/double)``, ' +
    'but [z](https://example.com/real) is a link.';
  assert.deepEqual(extractLinks(source), ['https://example.com/real']);
});

test('ignores http and relative links', () => {
  const source = [
    '[plain](http://example.com/insecure)',
    '[rel](../weeks/week-01/)',
    '[abs](/tracks/backend/)',
    '[mail](mailto:a@example.com)',
    'https://example.com/bare-url-not-a-link-target',
  ].join('\n');
  assert.deepEqual(extractLinks(source), []);
});

test('deduplicates in first-seen order and keeps fragments as written', () => {
  const source = [
    '[b](https://example.com/b)',
    '[a](https://example.com/a)',
    '[b again](https://example.com/b)',
    '[b frag](https://example.com/b#section)',
  ].join('\n');
  assert.deepEqual(extractLinks(source), [
    'https://example.com/b',
    'https://example.com/a',
    'https://example.com/b#section',
  ]);
});

test('keeps balanced parentheses and drops link titles', () => {
  const source =
    '[w](https://en.wikipedia.org/wiki/Function_(mathematics)) ' +
    '[t](https://example.com/t "A title")';
  assert.deepEqual(extractLinks(source), [
    'https://en.wikipedia.org/wiki/Function_(mathematics)',
    'https://example.com/t',
  ]);
});
