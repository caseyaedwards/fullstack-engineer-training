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
