import assert from 'node:assert/strict';
import { test } from 'node:test';
import { classify, errorCode, extractLinks, isTimeout } from '../../scripts/check-external-links.mjs';

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

// undici reports network errors as `TypeError: fetch failed` with the real error on `cause`.
const fetchFailed = (code, message = code) =>
  new TypeError('fetch failed', { cause: Object.assign(new Error(message), { code }) });
const level = (err) => classify({ error: errorCode(err), timeout: isTimeout(err) });

test('timeouts are warnings, wherever they come from', () => {
  for (const code of ['UND_ERR_CONNECT_TIMEOUT', 'UND_ERR_HEADERS_TIMEOUT', 'UND_ERR_BODY_TIMEOUT', 'ETIMEDOUT']) {
    const err = fetchFailed(code);
    assert.equal(errorCode(err), code);
    assert.equal(isTimeout(err), true, code);
    assert.equal(level(err), 'warn', code);
  }
  // Our own AbortSignal.timeout rejects with a DOMException named TimeoutError.
  const abort = new DOMException('The operation was aborted due to timeout', 'TimeoutError');
  assert.equal(errorCode(abort), 'TimeoutError');
  assert.equal(level(abort), 'warn');
  // A timeout two causes deep still counts.
  const deep = new TypeError('fetch failed', { cause: new Error('connect', { cause: Object.assign(new Error('t'), { code: 'ETIMEDOUT' }) }) });
  assert.equal(level(deep), 'warn');
});

test('DNS, refused, reset and TLS errors fail', () => {
  for (const code of ['ENOTFOUND', 'ECONNREFUSED', 'ECONNRESET', 'CERT_HAS_EXPIRED', 'ERR_TLS_CERT_ALTNAME_INVALID']) {
    const err = fetchFailed(code);
    assert.equal(errorCode(err), code);
    assert.equal(isTimeout(err), false, code);
    assert.equal(level(err), 'fail', code);
  }
  assert.equal(level(new TypeError('fetch failed')), 'fail');
});

test('status codes: 404 and 410 fail, other 4xx and 5xx warn', () => {
  assert.equal(classify({ status: 200 }), 'ok');
  assert.equal(classify({ status: 404 }), 'fail');
  assert.equal(classify({ status: 410 }), 'fail');
  for (const status of [401, 403, 429, 500, 503]) assert.equal(classify({ status }), 'warn', String(status));
});
