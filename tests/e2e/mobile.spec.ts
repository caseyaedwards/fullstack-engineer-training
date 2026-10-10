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
