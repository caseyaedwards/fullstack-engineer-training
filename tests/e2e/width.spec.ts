import { test, expect, type Page } from '@playwright/test';

const PAGE = 'weeks/04-networking/';

const column = (page: Page) =>
  page.locator('.sl-markdown-content').first().evaluate((el) => el.getBoundingClientRect().width);

async function pick(page: Page, value: string) {
  await page.getByLabel('Page width').first().selectOption(value);
}

test.describe('desktop', () => {
  test.use({ viewport: { width: 1800, height: 900 } });

  test('Slim < Medium < Wide < Full, and Medium is the default', async ({ page }) => {
    await page.goto(PAGE);
    await expect(page.getByLabel('Page width').first()).toHaveValue('medium');
    const medium = await column(page);
    await pick(page, 'slim');
    const slim = await column(page);
    await pick(page, 'wide');
    const wide = await column(page);
    await pick(page, 'full');
    const full = await column(page);
    expect(slim).toBeLessThan(medium);
    expect(wide).toBeGreaterThan(medium);
    expect(full).toBeGreaterThan(wide);
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(1800);
  });

  test('the choice survives a reload, with data-width set before first paint', async ({ page }) => {
    await page.goto(PAGE);
    await pick(page, 'wide');
    await expect(page.locator('html')).toHaveAttribute('data-width', 'wide');
    expect(await page.evaluate(() => localStorage.getItem('fsp-width'))).toBe('wide');
    await page.reload();
    await expect(page.locator('html')).toHaveAttribute('data-width', 'wide');
    await expect(page.getByLabel('Page width').first()).toHaveValue('wide');
  });

  test('an unknown stored value means Medium', async ({ page }) => {
    await page.addInitScript(() => localStorage.setItem('fsp-width', 'huge'));
    await page.goto(PAGE);
    await expect(page.getByLabel('Page width').first()).toHaveValue('medium');
    const w = await column(page);
    expect(w).toBeGreaterThan(600);
    expect(w).toBeLessThanOrEqual(720);
  });
});

test.describe('phone', () => {
  test.use({ viewport: { width: 390, height: 800 } });

  test('the width select sits in the mobile menu and the header does not overflow', async ({ page }) => {
    await page.goto(PAGE);
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
    await page.getByRole('button', { name: /menu/i }).click();
    const sel = page.locator('.mobile-preferences').getByLabel('Page width');
    await expect(sel).toBeVisible();
    const box = (await sel.boundingBox())!;
    expect(box.x + box.width).toBeLessThanOrEqual(390);
    await sel.selectOption('slim');
    await expect(page.locator('html')).toHaveAttribute('data-width', 'slim');
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  });
});
