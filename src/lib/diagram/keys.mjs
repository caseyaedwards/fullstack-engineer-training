// D2 tags every shape/edge <g> with a class that is the base64 of its
// HTML-escaped absolute ID, e.g. "YnJvd3Nlcg==" -> "browser" and
// base64("(browser -&gt; dns)[0]") for the first browser->dns edge.
// These helpers turn such a class into the short key authors write in
// <Diagram steps={[{ hl: [...] }]}>.

const ENTITIES = { '&gt;': '>', '&lt;': '<', '&amp;': '&', '&quot;': '"', '&#34;': '"', '&#39;': "'" };

function unescapeHtml(s) {
  return s.replace(/&(?:gt|lt|amp|quot|#34|#39);/g, (e) => ENTITIES[e]);
}

// "(a -> b)[n]" optionally scoped by a container path: "vpc.(a -> b)[n]".
const EDGE = /^(?:(.+)\.)?\((.+?) (->|<-|<->|--) (.+?)\)\[(\d+)\]$/;

/**
 * Normalize a raw D2 ID (HTML-escaped or not) to an author key:
 * shape `browser`, nested `vpc.api`, edge `a->b` (index 0 implied),
 * `a->b#1` for the second edge between the same pair, and container-scoped
 * edges fully qualified: `vpc.(a -> b)[0]` -> `vpc.a->vpc.b`.
 * @param {string} raw
 * @returns {string}
 */
export function normalizeKey(raw) {
  const id = unescapeHtml(raw);
  const m = EDGE.exec(id);
  if (!m) return id;
  const [, scope, src, op, dst, n] = m;
  const q = (s) => (scope ? `${scope}.${s}` : s);
  return `${q(src)}${op}${q(dst)}${n === '0' ? '' : `#${n}`}`;
}

/**
 * Decode a D2 element class into a normalized key, or null when the class is
 * not a D2 key class (e.g. "shape", "connection", "fill-B6").
 * @param {string} className
 * @returns {string | null}
 */
export function decodeKey(className) {
  if (!className || className.length % 4 !== 0 || !/^[A-Za-z0-9+/]+={0,2}$/.test(className)) return null;
  const buf = Buffer.from(className, 'base64');
  if (buf.toString('base64') !== className) return null;
  const raw = buf.toString('utf8');
  // Keys are printable text; reject binary noise from words that merely look like base64.
  if (!/^[^\p{C}]+$/u.test(raw) || raw.includes('�')) return null;
  return normalizeKey(raw);
}
