import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const html = readFileSync(new URL('../../dist/dev/diagrams/index.html', import.meta.url), 'utf8');

// rehype-mermaid (strategy: img-svg, dark: true) emits a <picture> whose
// <img> and dark <source> carry build-time SVG as data: URIs. An inline
// <svg id="mermaid-..."> is accepted too, in case the strategy changes.
test('mermaid block is rendered to SVG at build time', () => {
  const inlineSvg = /<svg[^>]*(id|class)="[^"]*mermaid[^"]*"/i.test(html);
  const dataSvg = /(?:src|srcset)="data:image\/svg\+xml[^"]*mermaid-\d+/i.test(html);
  assert.ok(inlineSvg || dataSvg, 'no Mermaid SVG found');
  assert.ok(!/<pre[^>]*data-language="mermaid"/.test(html), 'raw mermaid <pre> block remains');
});

test('dark-mode variant is present', () => {
  assert.match(
    html,
    /<picture>[\s\S]*?<source[^>]*media="\(prefers-color-scheme: dark\)"/,
    'no <picture> with a prefers-color-scheme: dark source',
  );
  assert.match(html, /mermaid-dark-\d+/, 'dark SVG not marked as Mermaid output');
});

test('no Mermaid JavaScript is shipped to readers', () => {
  assert.ok(!/mermaid(\.min)?\.m?js/i.test(html), 'mermaid script referenced');
  assert.ok(!/mermaid\.initialize/i.test(html), 'mermaid.initialize present');
});
