import { test } from 'node:test';
import assert from 'node:assert/strict';
import { checkHeading, frontmatterProse, lintSource, matchLine, strip } from '../lib/prose.mjs';

// The prose lint is only as good as its stripper and matchers. These tests pin both:
// what is hidden from the lint, what is still checked, and what each rule reports.

const words = (text) => matchLine(text).map((p) => p.word.toLowerCase());
const rules = (src, opts) => lintSource(src, opts).map((p) => `${p.line}:${p.rule}:${p.word}`);

test('matchLine finds banned words and hedges case-insensitively, on word boundaries only', () => {
  assert.deepEqual(words('The cache MAY hold it.'), ['may']);
  assert.deepEqual(words('It might work, and it usually does.'), ['might', 'usually']);
  assert.deepEqual(words('This tends to happen in most cases.'), ['tends to', 'in most cases']);
  assert.deepEqual(words('Kind of slow, sort of fast.'), ['kind of', 'sort of']);
  // Inside a longer word is not a match.
  assert.deepEqual(words('A maybelline mayor wasn here, unsorted, also goodwill, shoulder.'), []);
  assert.deepEqual(words('Use regex and/or click on it, in order to make sure.'), ['regex', 'and/or', 'click on', 'in order to', 'make sure']);
});

test('matchLine reports "may" once, though it is both banned and a hedge', () => {
  assert.equal(matchLine('It may fail.').length, 1);
});

test('matchLine allows the month May before a day, and a Go command-line flag', () => {
  assert.deepEqual(words('The certificate expires on May 5, 2027.'), []);
  assert.deepEqual(words('The flag package parses Go flags and command-line flags.'), ['flag']);
  assert.deepEqual(words('Set the flag in the UI.'), ['flag']);
});

test('matchLine finds contractions, including curly apostrophes, but not possessives', () => {
  assert.deepEqual(words("It isn't here, you're late, we've gone, they'll come, I'd go."), ["isn't", "you're", "we've", "they'll", "i'd"]);
  assert.deepEqual(words('It’s done and that’s it. Let’s go.'), ['it’s', 'that’s', 'let’s']);
  assert.deepEqual(words("The server's certificate and Go's client."), []);
  // A proper name: the certificate authority Let's Encrypt.
  assert.deepEqual(words("Let's Encrypt signs it. Let’s Encrypt it. Let's go."), ["let's"]);
});

test('matchLine finds e.g., i.e. and click here', () => {
  assert.deepEqual(words('Use a tool, e.g. curl, i.e. a client. Click here.'), ['e.g.', 'i.e.', 'click here']);
});

test('strip keeps line numbers and hides frontmatter, fences, imports, inline code and URLs', () => {
  const src = [
    '---', // 1
    'title: Might', // 2
    '---', // 3
    "import X from './x';", // 4
    '', // 5
    'Prose `may` and https://example.com/may here.', // 6
    '```go', // 7
    'x := 1 // would', // 8
    '```', // 9
    'Last [link text](https://example.com/should).', // 10
  ].join('\n');
  const { prose } = strip(src);
  const lines = prose.split('\n');
  assert.equal(lines.length, 10);
  assert.match(lines[5], /^Prose\s+and\s+here\.$/);
  assert.equal(lines[7].trim(), '');
  assert.match(lines[9], /Last\s+link text\s+\./);
  for (const n of [0, 1, 2, 3, 6, 7, 8]) assert.equal(lines[n].trim(), '', `line ${n + 1} should be blank`);
});

test('strip hides component tags and their props, but keeps the children of callouts', () => {
  const src = [
    '<ObserveIt>', // 1
    'Child text that might be checked.', // 2
    '</ObserveIt>', // 3
    '<Diagram', // 4
    '  id="x"', // 5
    "  title=\"A title that could fail\"", // 6
    '  code={`a -> b: would', // 7
    'b -> c: <not a tag>`}', // 8
    "  steps={[{ hl: ['a'], caption: 'It isn\\'t cached.' }]}", // 9
    '/>', // 10
    '{/* a comment that should hide */}', // 11
    'Text `<your module path>` stays prose.', // 12
  ].join('\n');
  const { prose, props } = strip(src);
  const lines = prose.split('\n');
  assert.equal(lines[1], 'Child text that might be checked.');
  for (const n of [0, 2, 3, 4, 5, 6, 7, 8, 9, 10]) assert.equal(lines[n].trim(), '', `line ${n + 1} should be blank`);
  assert.match(lines[11], /^Text\s+stays prose\.$/);
  // Visible string props are handed back separately, with their own line numbers, and D2 code is not.
  assert.deepEqual(props, [
    { line: 6, text: 'A title that could fail' },
    { line: 9, text: "It isn't cached." },
  ]);
});

test('strip unwraps emphasis so an emphasized word still matches', () => {
  assert.deepEqual(words(strip('It _may_ fail and **should** not.').prose), ['may', 'should']);
});

test('checkHeading accepts simple Title Case and flags each wrong word', () => {
  const code = new Set(['curl', 'dig', 'httptrace']);
  const bad = (h) => checkHeading(h, code).map((p) => p.word);
  assert.deepEqual(bad('Done When'), []);
  assert.deepEqual(bad('Done when'), ['when']);
  assert.deepEqual(bad('Observe it'), ['it']);
  assert.deepEqual(bad('In the Uptime Monitor'), []);
  assert.deepEqual(bad('In The Uptime Monitor'), ['The']);
  assert.deepEqual(bad('What to Look For'), []); // a small word is capitalized when it is last
  assert.deepEqual(bad('DNS: From a Name to an Address'), []); // and when it starts a segment after a colon
  assert.deepEqual(bad('Step 1: Meet httptrace'), []); // code names stay lowercase
  assert.deepEqual(bad('Trace With dig'), ['With']);
  assert.deepEqual(bad('Read trace.go and example.com'), []); // file names and hosts
  assert.deepEqual(bad('AI-Assisted Development'), []);
});

test('checkHeading rejects backticks, a trailing period, other punctuation and "N." numbering', () => {
  const rulesOf = (h) => checkHeading(h).map((p) => `${p.rule}:${p.word}`);
  assert.deepEqual(rulesOf('Meet `httptrace`'), ['heading-code:`']);
  assert.deepEqual(rulesOf('The End.'), ['heading-punctuation:.']);
  assert.deepEqual(rulesOf('Why Not?'), ['heading-punctuation:?']);
  assert.deepEqual(rulesOf('Foundations (Weeks 0 to 5)'), ['heading-punctuation:(', 'heading-punctuation:)']);
  assert.deepEqual(rulesOf('1. Meet Go'), ['heading-punctuation:1.']);
});

test('frontmatterProse returns title, description and done texts with their lines', () => {
  const src = ['---', 'title: "Week 1: Go"', 'description: It may run.', 'week: 1', 'done:', '  - id: a', '    text: You can say it.', '  - id: b', "    text: 'It''s done'", '---', 'Body'].join('\n');
  assert.deepEqual(frontmatterProse(src), [
    { line: 2, key: 'title', text: 'Week 1: Go' },
    { line: 3, key: 'description', text: 'It may run.' },
    { line: 7, key: 'done.text', text: 'You can say it.' },
    { line: 9, key: 'done.text', text: "It's done" },
  ]);
});

test('lintSource names the line of each problem across frontmatter, headings, prose, props and links', () => {
  const src = [
    '---', // 1
    'title: Start here', // 2
    'description: Perhaps.', // 3
    'done:', // 4
    '  - id: a', // 5
    "    text: It doesn't.", // 6
    '---', // 7
    '', // 8
    '## Done when', // 9
    '', // 10
    'The cache may hold it. See [here](https://example.com).', // 11
    '', // 12
    "<SideLab title=\"A lab you'll like\">", // 13
    '', // 14
    'Run `dig` e.g. twice.', // 15
    '', // 16
    '</SideLab>', // 17
    '```text', // 18
    "it may fail, it doesn't", // 19
    '```', // 20
  ].join('\n');
  assert.deepEqual(rules(src), [
    '2:title-case:here',
    '3:hedge:Perhaps',
    "6:contraction:doesn't",
    '9:title-case:when',
    '11:hedge:may',
    '11:link-text:here',
    "13:contraction:you'll",
    '15:latin:e.g.',
  ]);
});

test('matchLine bans the Axibase terms admin, deselect, uncheck, ingest, epoch time and datacenter', () => {
  assert.deepEqual(words('An admin can deselect or uncheck it, ingest logs, read Epoch time in the datacenter.'), [
    'admin', 'deselect', 'uncheck', 'ingest', 'epoch time', 'datacenter',
  ]);
});

test('matchLine finds the wider set of hedges and contractions', () => {
  assert.deepEqual(words('It sometimes, almost, mostly, normally and in general works.'), ['sometimes', 'almost', 'mostly', 'normally', 'in general']);
  assert.deepEqual(words('Commonly, frequently, occasionally, potentially, ideally, presumably, hopefully.'), [
    'commonly', 'frequently', 'occasionally', 'potentially', 'ideally', 'presumably', 'hopefully',
  ]);
  assert.deepEqual(words("How's it going, why's that, when's it due, y'all?"), ["how's", "why's", "when's", "y'all"]);
});

test('matchLine allows flag next to feature, SYN, ACK, FIN, RST, TCP, Go and command-line, and nothing else', () => {
  assert.deepEqual(words('Turn on the feature flag. The SYN flag, ACK flags, FIN flag, RST flag and TCP flags are set.'), []);
  assert.deepEqual(words('Go flags and command-line flags work.'), []);
  assert.deepEqual(words('Set the flag in the settings.'), ['flag']);
});

test('matchLine allows rather than, so far and if so, but not rather or so alone', () => {
  assert.deepEqual(words('Use TCP rather than UDP. So far it works; if so, stop.'), []);
  assert.deepEqual(words('It is rather slow, so it fails.'), ['rather', 'so']);
});

test('matchLine reads a capitalized May or Will after a lowercase word as a month or a name', () => {
  assert.deepEqual(words('The release ships in May, and Will reviews it.'), []);
  assert.deepEqual(words('The certificate expires on May 5, 2027.'), []);
  // At the start of a sentence, May and Will are the verbs.
  assert.deepEqual(words('May it fail? Will it work?'), ['may', 'will']);
});

test('matchLine keeps kill, terminate, sample and abort banned', () => {
  assert.deepEqual(words('Kill the process, terminate the job, sample a metric, abort the transaction.'), ['kill', 'terminate', 'sample', 'abort']);
});

test('a prose-allow comment suppresses the named words on its own line and the next line only', () => {
  const src = [
    '{/* prose-allow: kill, sample */}', // 1
    'Send kill to the process and take a sample.', // 2
    'Then kill it again.', // 3
    'Postgres marks the transaction as aborted. {/* prose-allow: aborted */}', // 4
    'Terminate it {/* prose-allow: kill */}', // 5
  ].join('\n');
  assert.deepEqual(rules(src), ['3:banned:kill', '5:banned:Terminate']);
});

test('frontmatterProse reads folded, literal and wrapped multi-line values, one entry per line', () => {
  const src = [
    '---', // 1
    'title: Week 1', // 2
    'description: >-', // 3
    '  It may run', // 4
    '  and it might stop.', // 5
    'done:', // 6
    '  - id: a', // 7
    '    text: |', // 8
    '      First line.', // 9
    '      Second line.', // 10
    '  - id: b', // 11
    '    text: A plain value that', // 12
    '      wraps here.', // 13
    'week: 1', // 14
    '---',
  ].join('\n');
  assert.deepEqual(frontmatterProse(src), [
    { line: 2, key: 'title', text: 'Week 1' },
    { line: 4, key: 'description', text: 'It may run' },
    { line: 5, key: 'description', text: 'and it might stop.' },
    { line: 9, key: 'done.text', text: 'First line.' },
    { line: 10, key: 'done.text', text: 'Second line.' },
    { line: 12, key: 'done.text', text: 'A plain value that' },
    { line: 13, key: 'done.text', text: 'wraps here.' },
  ]);
  assert.deepEqual(rules(src), ['4:hedge:may', '5:hedge:might']);
});

test('checkHeading allows mixed-case names and commas only in a list', () => {
  const bad = (h) => checkHeading(h).map((p) => `${p.rule}:${p.word}`);
  assert.deepEqual(bad('Call gRPC from an iPhone on macOS'), []);
  assert.deepEqual(bad('Addresses, Ports and Round Trips'), []);
  assert.deepEqual(bad('Tests, Builds or Releases'), []);
  assert.deepEqual(bad('Frontend vs Backend, Repo Skeleton'), ['heading-punctuation:,']);
});

test('strip hands back Badge text and card description props', () => {
  const { props } = strip('<Badge text="It would fail" />\n<LinkCard title="Go" description="A lot of it" />');
  assert.deepEqual(props, [
    { line: 1, text: 'It would fail' },
    { line: 2, text: 'Go' },
    { line: 2, text: 'A lot of it' },
  ]);
});
