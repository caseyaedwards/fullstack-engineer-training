// Build-time D2 -> inline, theme-able SVG.
//
// d2 renders with its default (light) theme and embeds rules like
// `.d2-123 .fill-B6{fill:#F7F8FE;}`. We rewrite each of those to
// `var(--d2-B6)` so src/styles/diagrams.css can map the palette onto
// Starlight's colour variables, which already follow the data-theme toggle.
import { spawn } from 'node:child_process';
import { createHash } from 'node:crypto';
import { fromHtml } from 'hast-util-from-html';
import { toHtml } from 'hast-util-to-html';
import { decodeKey, isLifeline } from './keys.mjs';

const d2Bin = () => process.env.D2_BIN ?? 'd2';
const d2TimeoutMs = () => Number(process.env.D2_TIMEOUT_MS) || 30_000;

/** @type {Map<string, Promise<{ svg: string, keys: Set<string> }>>} */
const cache = new Map();
/** @type {Promise<string> | undefined} */
let versionPromise;

function run(args, input) {
  return new Promise((resolve, reject) => {
    const D2 = d2Bin();
    const child = spawn(D2, args, { stdio: ['pipe', 'pipe', 'pipe'] });
    let out = '';
    let err = '';
    let settled = false;
    const done = (fn, v) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      fn(v);
    };
    const ms = d2TimeoutMs();
    const timer = setTimeout(() => {
      child.kill('SIGKILL');
      done(reject, new Error(`d2 timed out after ${ms} ms`));
    }, ms);
    child.stdout.setEncoding('utf8').on('data', (d) => (out += d));
    child.stderr.setEncoding('utf8').on('data', (d) => (err += d));
    child.on('error', (e) => done(reject, new Error(`d2: could not run "${D2}": ${e.message}`)));
    // d2 may exit before reading stdin (bad flags, crash): wait for 'close' to
    // report its stderr rather than crashing on EPIPE.
    child.stdin.on('error', () => {});
    child.on('close', (code) => {
      if (code === 0) done(resolve, out);
      else done(reject, new Error(`d2 exited with code ${code}: ${err.trim()}`));
    });
    child.stdin.end(input ?? '');
  });
}

function d2Version() {
  if (!versionPromise) {
    const p = run(['--version']).then((v) => v.trim());
    versionPromise = p;
    p.catch(() => {
      if (versionPromise === p) versionPromise = undefined;
    });
  }
  return versionPromise;
}

const THEME_RULE = /(\.(?:fill|stroke|color|background-color)-([A-Z]+\d+))\{([a-z-]+):#[0-9A-Fa-f]{3,8};\}/g;

/** Swap D2's hard-coded palette colours for CSS variables. */
function themeCss(svg) {
  return svg.replace(THEME_RULE, (_, sel, code, prop) => `${sel}{${prop}:var(--d2-${code});}`);
}

function* walk(node) {
  yield node;
  if (node.children) for (const c of node.children) yield* walk(c);
}

function classList(el) {
  const c = el.properties?.className;
  return Array.isArray(c) ? c.map(String) : typeof c === 'string' ? c.split(/\s+/).filter(Boolean) : [];
}

/**
 * Render D2 source to an inline SVG string plus the set of author keys it
 * contains. Cached in memory by sha256(source + d2 version + id).
 * @param {string} source
 * @param {{ id: string }} opts
 * @returns {Promise<{ svg: string, keys: Set<string> }>}
 */
export async function renderD2(source, { id }) {
  if (!id || !/^[A-Za-z][\w-]*$/.test(id)) throw new Error(`renderD2: invalid diagram id ${JSON.stringify(id)}`);
  const version = await d2Version();
  const hash = createHash('sha256').update(`${source}\0${version}\0${id}`).digest('hex');
  let p = cache.get(hash);
  if (!p) {
    p = render(source, id);
    cache.set(hash, p);
    p.catch(() => cache.delete(hash));
  }
  const { svg, keys } = await p;
  return { svg, keys: new Set(keys) };
}

async function render(source, id) {
  let raw;
  try {
    raw = await run(['--no-xml-tag', '--omit-version', '--pad', '0', '--salt', id, '-', '-'], source);
  } catch (e) {
    throw new Error(`D2 diagram "${id}" failed to render: ${e.message}`);
  }

  const tree = fromHtml(themeCss(raw.trim()), { fragment: true });
  const svgEl = tree.children.find((n) => n.type === 'element' && n.tagName === 'svg');
  if (!svgEl) throw new Error(`D2 diagram "${id}": d2 produced no <svg>`);

  // Explicit intrinsic size from the viewBox (outer svg only has a viewBox).
  const vb = String(svgEl.properties.viewBox ?? '').split(/[\s,]+/).map(Number);
  if (vb.length === 4 && vb.every(Number.isFinite)) {
    svgEl.properties.width = vb[2];
    svgEl.properties.height = vb[3];
  }

  const keys = new Set();
  const renamed = new Map();
  for (const node of walk(svgEl)) {
    if (node.type !== 'element') continue;
    // Shape/edge groups: <g class="<base64 key>">.
    if (node.tagName === 'g') {
      const cls = classList(node);
      const k = cls.length === 1 ? decodeKey(cls[0]) : null;
      if (k && !isLifeline(k)) {
        node.properties.dataK = k;
        keys.add(k);
      }
    }
    if (node.properties?.id != null) {
      const old = String(node.properties.id);
      const next = `${id}-${old}`;
      renamed.set(old, next);
      node.properties.id = next;
    }
  }

  // Re-point references (marker-end, mask, href, ...) at the prefixed ids.
  if (renamed.size) {
    const fixUrl = (s) => s.replace(/url\(#([^)\s]+)\)/g, (m, ref) => (renamed.has(ref) ? `url(#${renamed.get(ref)})` : m));
    const fixAttr = (s) => fixUrl(s).replace(/^#(\S+)$/, (m, ref) => (renamed.has(ref) ? `#${renamed.get(ref)}` : m));
    for (const node of walk(svgEl)) {
      if (node.type === 'text') node.value = fixUrl(node.value);
      if (node.type !== 'element') continue;
      for (const [name, value] of Object.entries(node.properties ?? {})) {
        if (typeof value === 'string' && value.includes('#')) node.properties[name] = fixAttr(value);
      }
    }
  }

  return { svg: toHtml(svgEl, { space: 'svg' }), keys };
}
