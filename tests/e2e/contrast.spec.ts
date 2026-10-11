import { test, expect, type Page } from '@playwright/test';

// WCAG contrast checks for the site theme, in both colour schemes. Colours are read
// with getComputedStyle and composited the way the browser paints them: every
// ancestor's background and opacity counts, so a dimmed diagram label or a disabled
// button is measured as it looks, not as its raw colour.

const WEEK = 'weeks/04-networking/';
const HOME = '';
const AA = 4.5;

type RGBA = [number, number, number, number];

// --- WCAG relative luminance (https://www.w3.org/TR/WCAG21/#dfn-relative-luminance) ---

const channel = (c: number) => {
  const s = c / 255;
  return s <= 0.04045 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
};
const luminance = ([r, g, b]: RGBA) => 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b);
const ratio = (a: RGBA, b: RGBA) => {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
};

function hue([r, g, b]: RGBA) {
  const [R, G, B] = [r / 255, g / 255, b / 255];
  const max = Math.max(R, G, B);
  const d = max - Math.min(R, G, B);
  if (d === 0) return NaN;
  const h = max === R ? ((G - B) / d) % 6 : max === G ? (B - R) / d + 2 : (R - G) / d + 4;
  return (h * 60 + 360) % 360;
}

// Runs in the page. Resolves a CSS colour to sRGB 0-255 plus alpha: parses rgb(),
// rgba() and color(srgb ...) directly, and paints anything else (oklch, lab, ...)
// onto a canvas to read it back.
function pageHelpers() {
  const w = window as unknown as Record<string, unknown>;
  if (w.__contrast) return;
  const canvas = document.createElement('canvas');
  canvas.width = canvas.height = 1;
  const ctx = canvas.getContext('2d', { willReadFrequently: true })!;

  const parse = (css: string): RGBA => {
    const s = css.trim();
    const num = (v: string, scale: number) => (v.endsWith('%') ? (parseFloat(v) / 100) * scale : parseFloat(v));
    let m = /^rgba?\(\s*([^)]+)\)$/i.exec(s);
    if (m) {
      const p = m[1].split(/[\s,/]+/).filter(Boolean);
      return [num(p[0], 255), num(p[1], 255), num(p[2], 255), p[3] === undefined ? 1 : num(p[3], 1)];
    }
    m = /^color\(\s*srgb\s+([^)]+)\)$/i.exec(s);
    if (m) {
      const p = m[1].split(/[\s/]+/).filter(Boolean);
      return [num(p[0], 1) * 255, num(p[1], 1) * 255, num(p[2], 1) * 255, p[3] === undefined ? 1 : num(p[3], 1)];
    }
    if (s === 'transparent' || s === 'none' || s === '') return [0, 0, 0, 0];
    ctx.clearRect(0, 0, 1, 1);
    ctx.fillStyle = '#000';
    ctx.fillStyle = s;
    ctx.fillRect(0, 0, 1, 1);
    const [r, g, b, a] = ctx.getImageData(0, 0, 1, 1).data;
    return [r, g, b, a / 255];
  };

  // Source-over: colour c (with alpha) painted on an opaque backdrop.
  const over = (c: RGBA, under: RGBA): RGBA => {
    const a = c[3];
    return [c[0] * a + under[0] * (1 - a), c[1] * a + under[1] * (1 - a), c[2] * a + under[2] * (1 - a), 1];
  };

  // A diagram label's own backdrop is the shape it sits in, if any.
  const ownBg = (el: Element): RGBA => {
    if (el instanceof SVGElement) {
      if (el.matches('g[data-k]')) {
        const shape = el.querySelector(':scope > g.shape > :is(rect, ellipse, polygon, path)');
        return shape ? parse(getComputedStyle(shape).fill) : [0, 0, 0, 0];
      }
      return [0, 0, 0, 0];
    }
    return parse(getComputedStyle(el).backgroundColor);
  };

  // Paint the chain root -> el and return the pixel under el's text. Each element's
  // background goes over the backdrop, its subtree is painted on that, and the result
  // is blended over the backdrop by the element's opacity (a group, as browsers do).
  const paint = (chain: Element[], text: RGBA | null): RGBA => {
    const go = (i: number, backdrop: RGBA): RGBA => {
      if (i === chain.length) return text ? over(text, backdrop) : backdrop;
      const el = chain[i];
      const inner = go(i + 1, over(ownBg(el), backdrop));
      const o = parseFloat(getComputedStyle(el).opacity);
      return over([inner[0], inner[1], inner[2], o], backdrop);
    };
    return go(0, [255, 255, 255, 1]);
  };

  w.__contrast = {
    parse,
    // Text colour and background as painted, for an HTML element or SVG <text>.
    pair(el: Element) {
      const chain: Element[] = [];
      for (let n: Element | null = el; n; n = n.parentElement) chain.unshift(n);
      const cs = getComputedStyle(el);
      const text = parse(el instanceof SVGTextElement ? cs.fill : cs.color);
      // Text colour alpha applies to the glyphs only.
      return { fg: paint(chain, text), bg: paint(chain, null) };
    },
    variable(name: string) {
      const probe = document.createElement('span');
      probe.style.color = `var(${name})`;
      document.body.append(probe);
      const c = parse(getComputedStyle(probe).color);
      probe.remove();
      return c;
    },
  };
}

async function load(page: Page, path: string, theme: 'dark' | 'light') {
  await page.addInitScript((t) => localStorage.setItem('starlight-theme', t), theme);
  await page.goto(path);
  await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
  await page.evaluate(pageHelpers);
}

const variable = (page: Page, name: string) =>
  page.evaluate((n) => (window as unknown as { __contrast: { variable(n: string): RGBA } }).__contrast.variable(n), name);

type Measured = { name: string; ratio: number; fg: RGBA; bg: RGBA };

async function measure(page: Page, name: string, selector: string): Promise<Measured[]> {
  const pairs = await page.locator(selector).evaluateAll((els) => {
    const c = (window as unknown as { __contrast: { pair(el: Element): { fg: RGBA; bg: RGBA } } }).__contrast;
    return els.map((el) => c.pair(el));
  });
  expect(pairs.length, `${name}: no element matches ${selector}`).toBeGreaterThan(0);
  return pairs.map((p) => ({ name, ratio: ratio(p.fg, p.bg), ...p }));
}

const worst = (ms: Measured[]) => ms.reduce((a, b) => (b.ratio < a.ratio ? b : a));
const fmt = (m: Measured) => `${m.name}: ${m.ratio.toFixed(2)}:1 (fg ${m.fg.slice(0, 3).map(Math.round)} on bg ${m.bg.slice(0, 3).map(Math.round)})`;

function expectAtLeast(ms: Measured[], min: number) {
  const m = worst(ms);
  test.info().annotations.push({ type: 'contrast', description: fmt(m) });
  expect.soft(m.ratio, fmt(m)).toBeGreaterThanOrEqual(min);
}

for (const theme of ['dark', 'light'] as const) {
  test.describe(`${theme} theme`, () => {
    test(`accent is green`, async ({ page }) => {
      await load(page, HOME, theme);
      // The blue tokens are remapped to the accent, so asides, badges and cards are green too.
      for (const name of ['--sl-color-accent-low', '--sl-color-accent', '--sl-color-accent-high', '--sl-color-blue']) {
        const h = hue(await variable(page, name));
        expect(h, `${name} hue ${h.toFixed(0)}`).toBeGreaterThanOrEqual(90);
        expect(h, `${name} hue ${h.toFixed(0)}`).toBeLessThanOrEqual(160);
      }
    });

    test(`week 4: text meets WCAG AA`, async ({ page }) => {
      await load(page, WEEK, theme);
      const fig = 'figure.dg#dns-lookup';
      await expect(page.locator(`${fig} .dg-nav [aria-live]`)).toContainText('1/');

      const checks: [string, string][] = [
        ['body text', '.sl-markdown-content > p'],
        ['sidebar link', '#starlight__sidebar a:not([aria-current="page"])'],
        ['sidebar current item', '#starlight__sidebar a[aria-current="page"]'],
        ['TOC link', 'starlight-toc a:not([aria-current="true"])'],
        ['step caption', `${fig} .dg-nav [aria-live]`],
        ['figcaption', `${fig} figcaption`],
        ['Prev button (disabled)', `${fig} .dg-nav button:disabled`],
        ['Next button', `${fig} .dg-nav button:enabled`],
        ['week header label', '.week-header dt'],
        ['callout text', '.callout p'],
        ['note aside text', '.starlight-aside--note p'],
        ['note aside title', '.starlight-aside--note .starlight-aside__title'],
        ['code token', '.expressive-code .ec-line span:not(:has(*))'],
      ];
      for (const [name, sel] of checks) expectAtLeast(await measure(page, name, sel), AA);

      const dimmed = await measure(page, 'dimmed diagram label', `${fig} .dg-dim text`);
      expectAtLeast(dimmed, AA);

      // A highlighted label reads clearly ahead of the dimmed ones.
      const lit = await measure(page, 'highlighted diagram label', `${fig} .dg-hl text`);
      expectAtLeast(lit, AA);
      expect.soft(worst(lit).ratio, 'highlighted labels stand out from dimmed ones').toBeGreaterThan(worst(dimmed).ratio * 1.5);
    });

    test(`home page: text meets WCAG AA`, async ({ page }) => {
      await load(page, HOME, theme);
      // Mark one week complete so the timeline's complete state is measured too.
      await page.locator('.week-timeline li[data-week]').first().evaluate((li) => li.setAttribute('data-complete', ''));

      const checks: [string, string][] = [
        ['body text', '.sl-markdown-content > p'],
        ['sidebar link', '#starlight__sidebar a:not([aria-current="page"])'],
        ['TOC link', 'starlight-toc a:not([aria-current="true"])'],
        ['primary button', '.sl-link-button.primary'],
        ['timeline week number', '.week-timeline .tl-n'],
        ['timeline title', '.week-timeline .tl-title'],
        ['timeline complete mark', '.week-timeline [data-complete] .tl-done'],
      ];
      for (const [name, sel] of checks) expectAtLeast(await measure(page, name, sel), AA);
    });
  });
}

// Printers drop backgrounds, so printed text sits on white paper whatever theme the
// reader picked on screen.
test(`print from dark mode: text meets WCAG AA on white`, async ({ page }) => {
  await page.emulateMedia({ media: 'print' });
  await load(page, WEEK, 'dark');
  const white: RGBA = [255, 255, 255, 1];
  for (const [name, sel] of [
    ['body text', '.sl-markdown-content > p'],
    ['link', '.sl-markdown-content p a'],
    ['heading', '.sl-markdown-content h2'],
  ]) {
    const fg = await page.locator(sel).first().evaluate((el) => getComputedStyle(el).color);
    const c = await page.evaluate((css) => (window as unknown as { __contrast: { parse(s: string): RGBA } }).__contrast.parse(css), fg);
    const r = ratio(c, white);
    expect.soft(r, `${name}: ${r.toFixed(2)}:1 (${fg})`).toBeGreaterThanOrEqual(AA);
  }
  const h = hue(await variable(page, '--sl-color-accent'));
  expect(h, `print --sl-color-accent hue ${h.toFixed(0)}`).toBeGreaterThanOrEqual(90);
  expect(h, `print --sl-color-accent hue ${h.toFixed(0)}`).toBeLessThanOrEqual(160);
});
