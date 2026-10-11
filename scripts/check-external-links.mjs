// Checks the external https:// links in the course pages.
//
// Run by .github/workflows/links.yml: the sandbox this repo is often edited
// in cannot reach these hosts, GitHub Actions runners can. Standard library
// only, so the workflow needs no npm install.
//
//   node scripts/check-external-links.mjs
//
// Exit 1 when a URL answers 404 or 410, or fails DNS, connection or TLS after
// the retries. 401, 403, 429 and other 5xx answers are warnings only: bot
// protection returns them for pages that are live. Timeouts are warnings too,
// whether ours (20 s) or undici's (a 10 s connect timeout, a headers or body
// timeout) or the kernel's (ETIMEDOUT): a slow host is not a dead link.
import { readFile, readdir } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const DOCS_DIR = path.join(ROOT, 'src', 'content', 'docs');
const SKIP_DIR = path.join(DOCS_DIR, 'dev');

const TIMEOUT_MS = 20_000;
const CONCURRENCY = 8;
const RETRY_WAITS_MS = [2_000, 5_000];
const USER_AGENT =
  'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36';

/**
 * Every https:// URL used as a Markdown link target, `](https://...)`, in an
 * MDX source. Fenced code blocks and inline code are skipped. Fragments are
 * kept as written. Deduplicated, in first-seen order.
 */
export function extractLinks(source) {
  const found = new Set();
  let fence = null; // { char, length } while inside a fenced code block

  for (const rawLine of source.split(/\r?\n/)) {
    const marker = /^\s*(`{3,}|~{3,})(.*)$/.exec(rawLine);
    if (fence) {
      // A closing fence uses the same character, is at least as long as the
      // opening one, and has nothing after it.
      if (
        marker &&
        marker[1][0] === fence.char &&
        marker[1].length >= fence.length &&
        marker[2].trim() === ''
      ) {
        fence = null;
      }
      continue;
    }
    if (marker) {
      fence = { char: marker[1][0], length: marker[1].length };
      continue;
    }

    // Inline code: a run of backticks up to the next run of the same length.
    const line = rawLine.replace(/(`+)(?!`)[\s\S]*?(?<!`)\1(?!`)/g, ' ');

    for (const match of line.matchAll(/\]\(\s*<?(https:\/\/)/g)) {
      const start = match.index + match[0].length - match[1].length;
      found.add(readTarget(line, start));
    }
  }
  return [...found];
}

// Reads a link target from `start`: up to whitespace, `>`, or the `)` that
// closes the link. Parentheses inside the URL are kept when balanced.
function readTarget(line, start) {
  let depth = 0;
  let end = start;
  for (; end < line.length; end++) {
    const ch = line[end];
    if (/\s/.test(ch) || ch === '>') break;
    if (ch === '(') depth++;
    else if (ch === ')') {
      if (depth === 0) break;
      depth--;
    }
  }
  return line.slice(start, end);
}

async function* walk(dir) {
  const entries = await readdir(dir, { withFileTypes: true });
  entries.sort((a, b) => a.name.localeCompare(b.name));
  for (const entry of entries) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (full === SKIP_DIR) continue;
      yield* walk(full);
    } else if (entry.isFile() && entry.name.endsWith('.mdx')) {
      yield full;
    }
  }
}

/** Map of URL to the files (relative to the repo root) that link to it. */
async function collectLinks() {
  const links = new Map();
  for await (const file of walk(DOCS_DIR)) {
    const rel = path.relative(ROOT, file).split(path.sep).join('/');
    for (const url of extractLinks(await readFile(file, 'utf8'))) {
      if (!links.has(url)) links.set(url, []);
      links.get(url).push(rel);
    }
  }
  return links;
}

// Error codes that mean the host was slow, not gone. undici reports its own
// timeouts as `TypeError: fetch failed` with the code on `err.cause`.
const TIMEOUT_CODES = new Set([
  'UND_ERR_CONNECT_TIMEOUT',
  'UND_ERR_HEADERS_TIMEOUT',
  'UND_ERR_BODY_TIMEOUT',
  'ETIMEDOUT',
]);

/** The errors on the chain err, err.cause, err.cause.cause, ... */
function* causes(err) {
  for (let e = err, depth = 0; e != null && depth < 5; e = e.cause, depth++) yield e;
}

/** The most specific code on a fetch error: the deepest `code`, else the name. */
export function errorCode(err) {
  let code;
  for (const e of causes(err)) if (typeof e.code === 'string') code = e.code;
  return code ?? err?.name ?? 'ERROR';
}

/** True when a fetch error is a timeout: our AbortSignal, or a timeout code anywhere on the cause chain. */
export function isTimeout(err) {
  for (const e of causes(err)) {
    if (e.name === 'TimeoutError' || TIMEOUT_CODES.has(e.code)) return true;
  }
  return false;
}

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

// One request. Resolves { status } or { error }; never throws.
async function fetchOnce(url) {
  try {
    const response = await fetch(url, {
      method: 'GET',
      redirect: 'follow',
      signal: AbortSignal.timeout(TIMEOUT_MS),
      headers: {
        'User-Agent': USER_AGENT,
        Accept: 'text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8',
        'Accept-Language': 'en-US,en;q=0.9',
      },
    });
    // The status is all that matters; do not download the page.
    await response.body?.cancel().catch(() => {});
    return { status: response.status };
  } catch (err) {
    return { error: String(errorCode(err)), timeout: isTimeout(err) };
  }
}

async function checkUrl(url) {
  let result;
  for (let attempt = 0; ; attempt++) {
    result = await fetchOnce(url);
    const retryable = result.error !== undefined || result.status === 429 || result.status >= 500;
    if (!retryable || attempt >= RETRY_WAITS_MS.length) break;
    await sleep(RETRY_WAITS_MS[attempt]);
  }
  return { ...result, level: classify(result) };
}

/** ok, warn or fail for one result: { status } or { error, timeout }. */
export function classify({ status, error, timeout }) {
  if (error !== undefined) {
    // A slow host is not a dead link: report timeouts, but do not fail.
    return timeout ? 'warn' : 'fail';
  }
  if (status === 404 || status === 410) return 'fail';
  if (status >= 400) return 'warn'; // 401, 403, 429, other 4xx, 5xx
  return 'ok';
}

async function mapLimit(items, limit, fn) {
  const results = new Array(items.length);
  let next = 0;
  const workers = Array.from({ length: Math.min(limit, items.length) }, async () => {
    while (next < items.length) {
      const i = next++;
      results[i] = await fn(items[i]);
    }
  });
  await Promise.all(workers);
  return results;
}

async function main() {
  const links = await collectLinks();
  const urls = [...links.keys()];
  console.log(`Checking ${urls.length} unique external links.`);

  const counts = { ok: 0, warn: 0, fail: 0 };
  await mapLimit(urls, CONCURRENCY, async (url) => {
    const result = await checkUrl(url);
    counts[result.level]++;
    const label = result.error ?? String(result.status);
    const files = links.get(url).join(', ');
    console.log(`${result.level.toUpperCase().padEnd(4)} ${label} ${url} ${files}`);
  });

  console.log(
    `\nSummary: ${counts.ok} ok, ${counts.warn} warnings, ${counts.fail} failures.`,
  );
  return counts.fail > 0 ? 1 : 0;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  process.exitCode = await main();
}
