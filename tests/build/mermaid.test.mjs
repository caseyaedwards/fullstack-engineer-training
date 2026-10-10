import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';

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
  assert.match(html, /mermaid-dark-\d+/, 'dark SVG not marked as Mermaid output');
});

test('no Mermaid JavaScript is shipped to readers', () => {
  assert.ok(!/mermaid(\.min)?\.m?js/i.test(html), 'mermaid script referenced');
  assert.ok(!/mermaid\.initialize/i.test(html), 'mermaid.initialize present');
});

test('light and dark variants are classed <img>s that follow data-theme', () => {
  assert.ok(!/<picture/.test(html), '<picture> should have been rewritten');
  assert.match(html, /<img[^>]*class="diagram-light"[^>]*>/, 'no diagram-light img');
  assert.match(html, /<img[^>]*class="diagram-dark"[^>]*>/, 'no diagram-dark img');
  const darkImg = html.match(/<img[^>]*class="diagram-dark"[^>]*>/)[0];
  assert.match(darkImg, /src="data:image\/svg\+xml[^"]*mermaid-dark/, 'dark img lacks dark SVG');
  const dir = new URL('../../dist/_astro/', import.meta.url);
  const css = readdirSync(dir)
    .filter((f) => f.endsWith('.css'))
    .map((f) => readFileSync(new URL(f, dir), 'utf8'))
    .join('\n');
  // The CSS minifier may merge the two rules into one comma-separated rule.
  assert.match(css, /:root\[data-theme=['"]?dark['"]?\]\s*\.diagram-light\s*[,{]/, 'dark theme must hide .diagram-light');
  assert.match(css, /:root:not\(\[data-theme=['"]?dark['"]?\]\)\s*\.diagram-dark\s*[,{]/, 'non-dark theme must hide .diagram-dark');
  assert.match(css, /\.diagram-(?:light|dark)\s*\{[^}]*display:\s*none/, 'rules must set display:none');
});
