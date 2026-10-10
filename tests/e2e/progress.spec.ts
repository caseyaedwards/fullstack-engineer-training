import { test, expect, type Page } from '@playwright/test';

const WEEK = 'weeks/04-networking/';
const boxes = (page: Page) => page.locator('.done-when input[type="checkbox"]');
const node = (page: Page, week: number) => page.locator(`.week-timeline li[data-week="${week}"]`);

async function tickAll(page: Page) {
  const n = await boxes(page).count();
  expect(n).toBe(3);
  for (let i = 0; i < n; i++) await boxes(page).nth(i).check();
}

test('ticked items survive a reload and complete the week on the home timeline', async ({ page }) => {
  await page.goto(WEEK);
  await expect(boxes(page).first()).not.toBeChecked();
  await tickAll(page);

  await page.reload();
  for (let i = 0; i < 3; i++) await expect(boxes(page).nth(i)).toBeChecked();

  await page.goto('');
  await expect(node(page, 4)).toHaveAttribute('data-complete', '');
  await expect(node(page, 4).locator('.tl-done')).toBeVisible();
  await expect(node(page, 3)).not.toHaveAttribute('data-complete', '');
  await expect(node(page, 3).locator('.tl-done')).toBeHidden();
});

test('a partly ticked week is not complete, and unticking undoes completion', async ({ page }) => {
  await page.goto(WEEK);
  await boxes(page).nth(0).check();
  await boxes(page).nth(1).check();
  await page.goto('');
  await expect(node(page, 4)).not.toHaveAttribute('data-complete', '');

  await page.goto(WEEK);
  await boxes(page).nth(2).check();
  await page.goto('');
  await expect(node(page, 4)).toHaveAttribute('data-complete', '');

  await page.goto(WEEK);
  await boxes(page).nth(2).uncheck();
  await page.goto('');
  await expect(node(page, 4)).not.toHaveAttribute('data-complete', '');
});

test('the timeline lists 19 weeks in 4 phases, each linking to its week', async ({ page, baseURL }) => {
  await page.goto('');
  await expect(page.locator('.week-timeline li[data-week]')).toHaveCount(19);
  await expect(page.locator('.week-timeline [data-phase]')).toHaveCount(4);
  const href = await node(page, 4).locator('a').getAttribute('href');
  expect(new URL(href!, baseURL).pathname).toBe(new URL('weeks/04-networking/', baseURL).pathname);
});

test('the start link goes to week 0', async ({ page }) => {
  await page.goto('');
  await page.getByRole('link', { name: 'Start at week 0' }).click();
  await expect(page).toHaveURL(/weeks\/00-setup\/$/);
});

test.describe('when storage throws (private window)', () => {
  test('week renders, ticking works for the session, and no page error is logged', async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', (e) => errors.push(e.message));
    page.on('console', (m) => m.type() === 'error' && errors.push(m.text()));
    // Only our keys throw. Starlight's own theme code reads localStorage unguarded, so a
    // blanket throw would log its errors, which this site does not own.
    await page.addInitScript(() => {
      for (const method of ['getItem', 'setItem'] as const) {
        const original = Storage.prototype[method];
        Storage.prototype[method] = function (this: Storage, key: string, ...rest: string[]) {
          if (key.startsWith('fsp:')) throw new DOMException('denied', 'SecurityError');
          return (original as (...a: string[]) => string | null | void).call(this, key, ...rest);
        } as typeof original;
      }
    });

    await page.goto(WEEK);
    await expect(page.locator('.week-header')).toBeVisible();
    await tickAll(page);
    for (let i = 0; i < 3; i++) await expect(boxes(page).nth(i)).toBeChecked();
    await boxes(page).nth(1).uncheck();
    await expect(boxes(page).nth(1)).not.toBeChecked();

    await page.goto('');
    await expect(page.locator('.week-timeline li[data-week]')).toHaveCount(19);
    expect(errors).toEqual([]);
  });
});
