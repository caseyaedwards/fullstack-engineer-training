// Prose lint for the site copy. It enforces the machine-checkable rules of
// docs/style-guide.md. The build test (tests/build/prose.test.mjs) runs it over every
// page, and tests/unit/prose.test.mjs checks the stripper and the matchers themselves.
//
// The stripper turns an .mdx source into prose with the same line numbers: code,
// JSX tags and their props, imports, URLs and frontmatter become spaces, and the
// children of components stay, so callout text is still checked.

/** Words and phrases the guide bans, with what to write instead. Matched case-insensitively on word boundaries. */
export const BANNED = [
  { words: ['should'], fix: 'must, or remove' },
  { words: ['could', 'would'], fix: 'rewrite to state what happens' },
  // "May" followed by a number is the month, as in "May 5".
  { words: ['may'], fix: 'can', except: /\bMay\s+\d/g },
  { words: ['will', 'was', 'were', 'had'], fix: 'present tense' },
  { words: ['abort', 'aborts', 'aborted', 'kill', 'kills', 'killed', 'terminate', 'terminates', 'terminated'], fix: 'stop or cancel' },
  { words: ['so', 'a lot'], fix: 'formal style: therefore, because, to, many' },
  // A Go command-line flag is the correct term for the flag package.
  { words: ['flag', 'flags'], fix: 'option', except: /\b(?:Go|command-line)\s+flags?\b/gi },
  { words: ['lets', 'please'], fix: 'remove' },
  { words: ['regex', 'regexes'], fix: 'regular expression' },
  { words: ['and/or'], fix: 'clarify: and, or, or one of' },
  { words: ['in order to'], fix: 'to' },
  { words: ['make sure'], fix: 'ensure' },
  { words: ['end-point'], fix: 'endpoint' },
  { words: ['click on'], fix: 'click' },
  { words: ['robust'], fix: 'say what it withstands' },
  { words: ['sample', 'samples'], fix: 'example' },
  { words: ['follow these steps'], fix: 'remove the filler' },
];

/** Conditional and hedging language. Rewrite the sentence to state what happens, with a number, condition or example. */
export const HEDGES = [
  'may', 'might', 'perhaps', 'maybe', 'possibly', 'probably', 'likely', 'could', 'would', 'should',
  'generally', 'usually', 'typically', 'often', 'somewhat', 'fairly', 'quite', 'rather', 'arguably',
  'tends to', 'tend to', 'in most cases', 'it depends', 'kind of', 'sort of', 'seem', 'seems', 'seemed', 'appears to', 'appear to',
];

/** Words that stay lowercase in Title Case unless they are the first or last word. */
export const SMALL_WORDS = new Set(
  ['a', 'an', 'the', 'and', 'but', 'or', 'for', 'nor', 'on', 'at', 'to', 'by', 'of', 'in', 'with', 'from', 'as', 'vs'],
);

const escape = (s) => s.replace(/[.*+?^${}()|[\]\\/]/g, '\\$&');
const wordRegex = (w) => new RegExp(`(?<![\\w'’-])${w.split(' ').map(escape).join('\\s+')}(?![\\w'’-])`, 'gi');

const RULES = [
  ...HEDGES.map((w) => ({ re: wordRegex(w), rule: 'hedge', fix: 'state what happens, with a number, condition or example', except: w === 'may' ? /\bMay\s+\d/g : undefined })),
  ...BANNED.flatMap(({ words, fix, except }) =>
    words.filter((w) => !HEDGES.includes(w)).map((w) => ({ re: wordRegex(w), rule: 'banned', fix, except })),
  ),
  { re: /\b\w+n['’]t\b/gi, rule: 'contraction', fix: 'write it out: is not, do not' },
  { re: /\b\w+['’](?:re|ve|ll|d)\b/gi, rule: 'contraction', fix: 'write it out' },
  { re: /\b(?:it|that|let|here|there|what|who|where|he|she)['’]s\b/gi, rule: 'contraction', fix: 'write it out: it is, that is' },
  { re: /\b(?:e\.g|i\.e)\.?/gi, rule: 'latin', fix: 'for example' },
  { re: /\bclick\s+here\b/gi, rule: 'link-text', fix: 'describe the target' },
];

/** Every problem in one line of prose: [{ word, rule, fix, col }]. */
export function matchLine(text) {
  const found = [];
  for (const { re, rule, fix, except } of RULES) {
    const allowed = except ? [...text.matchAll(except)].map((m) => [m.index, m.index + m[0].length]) : [];
    for (const m of text.matchAll(re)) {
      if (allowed.some(([a, b]) => m.index >= a && m.index < b)) continue;
      found.push({ word: m[0], rule, fix, col: m.index + 1 });
    }
  }
  // "may" and the like are both banned and hedges: report each place once.
  const seen = new Set();
  return found
    .sort((a, b) => a.col - b.col)
    .filter((p) => {
      const key = `${p.col}:${p.word.toLowerCase()}`;
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    });
}

// ---------------------------------------------------------------------------
// Stripper

const blank = (s) => s.replace(/[^\n]/g, ' ');

/** Index just past the quoted string that opens at i (', " or `), honoring backslash escapes. */
function skipString(src, i) {
  const q = src[i];
  let j = i + 1;
  while (j < src.length) {
    if (src[j] === '\\') j += 2;
    else if (src[j] === q) return j + 1;
    else j++;
  }
  return src.length;
}

/** Index just past the `}` that closes the `{` at i. */
function skipBraces(src, i) {
  let depth = 0;
  let j = i;
  while (j < src.length) {
    const c = src[j];
    if (c === '"' || c === "'" || c === '`') {
      j = skipString(src, j);
      continue;
    }
    if (c === '{') depth++;
    else if (c === '}' && --depth === 0) return j + 1;
    j++;
  }
  return src.length;
}

/** Index just past the `>` that closes the JSX or HTML tag opening at i. */
function skipTag(src, i) {
  let j = i + 1;
  while (j < src.length) {
    const c = src[j];
    if (c === '"' || c === "'" || c === '`') j = skipString(src, j);
    else if (c === '{') j = skipBraces(src, j);
    else if (c === '>') return j + 1;
    else j++;
  }
  return src.length;
}

/** String props that render as visible text: Diagram titles and step captions, SideLab titles. */
function propText(tag, offset, src) {
  // Template literals hold D2 code, not prose: hide them first.
  const t = tag.replace(/`(?:\\.|[^`\\])*`/g, blank);
  const out = [];
  for (const m of t.matchAll(/\b(title|caption)\s*[:=]\s*(['"])((?:\\.|(?!\2)[^\\])*)\2/g)) {
    const at = offset + m.index;
    out.push({ line: src.slice(0, at).split('\n').length, text: m[3].replace(/\\(.)/g, '$1') });
  }
  return out;
}

/**
 * Hides everything that is not prose, keeping line numbers.
 * Returns { prose, headings, props, links }:
 *   prose     the text with code, tags, imports, URLs and frontmatter blanked;
 *   headings  [{ line, raw }] markdown headings, with inline code still in place;
 *   props     [{ line, text }] visible string props of components;
 *   links     [{ line, text }] markdown link texts.
 */
export function strip(src) {
  let text = src;

  // Frontmatter.
  const fm = /^---\n[\s\S]*?\n---(?=\n|$)/.exec(text);
  if (fm) text = blank(fm[0]) + text.slice(fm[0].length);

  // Fenced code blocks, import and export lines, and :::note directive markers.
  const lines = text.split('\n');
  let fence = null;
  for (let i = 0; i < lines.length; i++) {
    const open = /^\s*(`{3,}|~{3,})/.exec(lines[i]);
    if (fence) {
      if (open && open[1][0] === fence[0] && open[1].length >= fence.length && /^\s*[`~]+\s*$/.test(lines[i])) fence = null;
      lines[i] = blank(lines[i]);
    } else if (open) {
      fence = open[1];
      lines[i] = blank(lines[i]);
    } else if (/^(?:import|export)\s/.test(lines[i]) || /^:::/.test(lines[i])) {
      lines[i] = blank(lines[i]);
    }
  }
  text = lines.join('\n');

  // One pass for comments, JSX expressions, tags and inline code, so a backtick inside a
  // tag prop is not taken for inline code, and a `<` inside inline code is not taken for a tag.
  const props = [];
  let noTags = '';
  let prose = '';
  let i = 0;
  while (i < text.length) {
    const c = text[i];
    let end = i;
    let kind = null;
    if (text.startsWith('<!--', i)) {
      const e = text.indexOf('-->', i + 4);
      end = e === -1 ? text.length : e + 3;
      kind = 'tag';
    } else if (c === '<' && /[A-Za-z]/.test(text[i + 1] === '/' ? text[i + 2] ?? '' : text[i + 1] ?? '')) {
      end = skipTag(text, i);
      props.push(...propText(text.slice(i, end), i, text));
      kind = 'tag';
    } else if (c === '{') {
      end = skipBraces(text, i);
      kind = 'tag';
    } else if (c === '`') {
      const run = /^`+/.exec(text.slice(i))[0];
      const close = text.indexOf(run, i + run.length);
      // A run of the same length closes the span; a longer run does not.
      if (close !== -1 && text[close + run.length] !== '`') {
        end = close + run.length;
        kind = 'code';
      } else {
        end = i + run.length;
      }
    } else if (c === '\\') {
      end = i + 2;
    } else {
      end = i + 1;
    }
    const chunk = text.slice(i, end);
    noTags += kind === 'tag' ? blank(chunk) : chunk;
    prose += kind ? blank(chunk) : chunk;
    i = end;
  }

  const headings = [];
  noTags.split('\n').forEach((line, n) => {
    const h = /^#{1,6}\s+(.*?)\s*#*\s*$/.exec(line);
    if (h) headings.push({ line: n + 1, raw: h[1] });
  });

  // Markdown links: keep the text, check it, drop the target. Then bare URLs.
  const links = [];
  prose = prose
    .split('\n')
    .map((line, n) =>
      line
        .replace(/\[([^\]]*)\]\(([^)]*)\)/g, (m, label, target) => {
          links.push({ line: n + 1, text: label });
          return ` ${label} ${blank(target)} `;
        })
        .replace(/^\s*\[[^\]]+\]:\s*\S+.*$/, blank)
        .replace(/\b(?:https?|ftp|mailto):[^\s)>\]]+/g, blank)
        // Emphasis markers are not words; `_where_` must still match "where".
        .replace(/[*_]/g, ' '),
    )
    .join('\n');

  return { prose, headings, props, links };
}

// ---------------------------------------------------------------------------
// Headings

/**
 * Problems with one heading or page title: Title Case, backticks and punctuation.
 * `codeWords` are lowercase program or code names allowed to stay lowercase, such as curl.
 */
export function checkHeading(raw, codeWords = new Set()) {
  const problems = [];
  if (raw.includes('`')) problems.push({ rule: 'heading-code', word: '`', fix: 'no backticks in headings' });
  const text = raw
    .replace(/`[^`]*`/g, ' ')
    .replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/[*_]/g, '');
  if (/\.\s*$/.test(text)) problems.push({ rule: 'heading-punctuation', word: '.', fix: 'no trailing period' });
  for (const m of text.matchAll(/[?!;()"“”]/g)) problems.push({ rule: 'heading-punctuation', word: m[0], fix: 'colons only' });
  if (/^\d+\.\s/.test(text)) problems.push({ rule: 'heading-punctuation', word: text.split(' ')[0], fix: 'write Step N: instead of N.' });

  for (const segment of text.split(':')) {
    const tokens = segment.trim().split(/\s+/).filter(Boolean);
    tokens.forEach((token, idx) => {
      const word = token.replace(/^[^\p{L}\p{N}+-]+|[^\p{L}\p{N}]+$/gu, '');
      if (!word || !/^\p{L}/u.test(word)) return; // numbers, +trace, punctuation
      if (codeWords.has(word)) return; // a program or code name keeps its case
      if (/\p{L}[./]\p{L}/u.test(word) && word === word.toLowerCase()) return; // file names and hosts: trace.go, example.com
      const edge = idx === 0 || idx === tokens.length - 1;
      const small = SMALL_WORDS.has(word.toLowerCase());
      const upper = word[0] !== word[0].toLowerCase();
      if (small && !edge && upper) {
        problems.push({ rule: 'title-case', word, fix: `small word stays lowercase: ${word.toLowerCase()}` });
      } else if ((!small || edge) && !upper) {
        problems.push({ rule: 'title-case', word, fix: `capitalize: ${word[0].toUpperCase()}${word.slice(1)}` });
      }
    });
  }
  return problems;
}

// ---------------------------------------------------------------------------
// Frontmatter

const unquote = (s) => {
  const t = s.trim();
  if (/^".*"$/.test(t)) return JSON.parse(t);
  if (/^'.*'$/.test(t)) return t.slice(1, -1).replaceAll("''", "'");
  return t;
};

/** The prose strings in frontmatter: [{ line, key, text }] for title, description and done[].text. */
export function frontmatterProse(src) {
  const fm = /^---\n([\s\S]*?)\n---/.exec(src);
  if (!fm) return [];
  const out = [];
  let inDone = false;
  fm[1].split('\n').forEach((line, n) => {
    const top = /^([A-Za-z][\w-]*):\s*(.*)$/.exec(line);
    if (top) {
      inDone = top[1] === 'done';
      if (top[1] === 'title' || top[1] === 'description') out.push({ line: n + 2, key: top[1], text: unquote(top[2]) });
      return;
    }
    const item = /^\s+(?:-\s+)?text:\s*(.*)$/.exec(line);
    if (inDone && item) out.push({ line: n + 2, key: 'done.text', text: unquote(item[1]) });
  });
  return out;
}

// ---------------------------------------------------------------------------
// Whole file

/** Every problem in one .mdx source: [{ line, col?, word, rule, fix }], sorted by line. */
export function lintSource(src, { codeWords = new Set() } = {}) {
  const problems = [];
  const add = (line, list) => problems.push(...list.map((p) => ({ line, ...p })));

  for (const { line, key, text } of frontmatterProse(src)) {
    add(line, matchLine(text).map((p) => ({ ...p, col: undefined, where: key })));
    if (key === 'title') add(line, checkHeading(text, codeWords).map((p) => ({ ...p, where: key })));
  }

  const { prose, headings, props, links } = strip(src);
  prose.split('\n').forEach((line, n) => add(n + 1, matchLine(line)));
  for (const { line, raw } of headings) add(line, checkHeading(raw, codeWords));
  for (const { line, text } of props) add(line, matchLine(text).map((p) => ({ ...p, col: undefined, where: 'prop' })));
  for (const { line, text } of links) {
    if (/^\s*here\s*$/i.test(text)) add(line, [{ word: text.trim(), rule: 'link-text', fix: 'describe the target' }]);
  }
  return problems.sort((a, b) => a.line - b.line || (a.col ?? 1e9) - (b.col ?? 1e9));
}

/** One line per problem: file:line[:col]: rule "word" (fix). */
export const formatProblem = (file, p) =>
  `${file}:${p.line}${p.col ? `:${p.col}` : ''}: ${p.rule} "${p.word}"${p.where ? ` in ${p.where}` : ''} (${p.fix})`;
