import { test, expect } from '@playwright/test';

test.use({ viewport: { width: 375, height: 800 } });

const PAGES = [
  ['diagram fixture', 'dev/diagrams/'],
  ['week 4', 'weeks/04-networking/'],
  ['track page', 'tracks/foundations/networking/'],
  ['home page', ''],
];

for (const [name, path] of PAGES) {
  test(`no horizontal page scroll at 375px: ${name}`, async ({ page }) => {
    await page.goto(path);
    const width = await page.evaluate(() => document.documentElement.scrollWidth);
    expect(width).toBeLessThanOrEqual(375);
  });
}

test('a wide table scrolls inside its own box at 375px, not the page', async ({ page }) => {
  await page.goto('dev/diagrams/');
  const m = await page.evaluate(() => {
    const t = document.querySelector('.sl-markdown-content table')!;
    return { table: [t.scrollWidth, t.clientWidth], page: document.documentElement.scrollWidth };
  });
  expect(m.table[0], 'table should be wider than its box').toBeGreaterThan(m.table[1]);
  expect(m.page).toBeLessThanOrEqual(375);
});

test('at 375px a wide diagram stays legible and scrolls in its box; a narrow one still fits', async ({ page }) => {
  await page.goto('weeks/04-networking/');
  const size = (id: string) =>
    page.locator(`figure.dg#${id} .dg-svg`).evaluate((box) => {
      const svg = box.querySelector('svg')!;
      return { natural: Number(svg.getAttribute('width')), drawn: svg.getBoundingClientRect().width, box: box.clientWidth, scroll: box.scrollWidth };
    });
  const wide = await size('dns-lookup');
  expect(wide.natural, 'fixture: the DNS diagram is wider than the phone').toBeGreaterThan(wide.box / 0.75);
  expect(wide.drawn).toBeGreaterThanOrEqual(wide.natural * 0.75 - 1);
  expect(wide.scroll).toBeGreaterThan(wide.box);

  const narrow = await size('request-path');
  expect(narrow.drawn).toBeLessThanOrEqual(narrow.box);
  expect(narrow.scroll).toBeLessThanOrEqual(narrow.box);
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(375);
});
