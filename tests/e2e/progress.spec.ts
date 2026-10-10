import { test, expect, type Page } from '@playwright/test';

const WEEK = 'weeks/04-networking/';
const boxes = (page: Page) => page.locator('.done-when input[type="checkbox"]');
const node = (page: Page, week: number) => page.locator(`.week-timeline li[data-week="${week}"]`);

/** Ticks every box on a week page and returns how many there were. */
async function tickAll(page: Page) {
  const n = await boxes(page).count();
  expect(n, 'checkbox count matches data-total').toBe(Number(await page.locator('.done-when').getAttribute('data-total')));
  for (let i = 0; i < n; i++) await boxes(page).nth(i).check();
  return n;
}

/** The week's current done ids, as the home timeline knows them. */
const idsOf = async (page: Page, week: number): Promise<string[]> => JSON.parse((await node(page, week).getAttribute('data-ids'))!);

test('ticked items survive a reload and complete the week on the home timeline', async ({ page }) => {
  await page.goto(WEEK);
  await expect(boxes(page).first()).not.toBeChecked();
  const n = await tickAll(page);

  await page.reload();
  for (let i = 0; i < n; i++) await expect(boxes(page).nth(i)).toBeChecked();

  await page.goto('');
  await expect(node(page, 4)).toHaveAttribute('data-complete', '');
  await expect(node(page, 4).locator('.tl-done')).toBeVisible();
  await expect(node(page, 3)).not.toHaveAttribute('data-complete', '');
  await expect(node(page, 3).locator('.tl-done')).toBeHidden();
});

test('a partly ticked week is not complete, and unticking undoes completion', async ({ page }) => {
  await page.goto(WEEK);
  const last = (await boxes(page).count()) - 1;
  for (let i = 0; i < last; i++) await boxes(page).nth(i).check();
  await page.goto('');
  await expect(node(page, 4)).not.toHaveAttribute('data-complete', '');

  await page.goto(WEEK);
  await boxes(page).nth(last).check();
  await page.goto('');
  await expect(node(page, 4)).toHaveAttribute('data-complete', '');

  await page.goto(WEEK);
  await boxes(page).nth(last).uncheck();
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
  await page.getByRole('link', { name: 'Start at Week 0' }).click();
  await expect(page).toHaveURL(/weeks\/00-setup\/$/);
});

test('only ids in the week\'s current list count toward completion', async ({ page }) => {
  await page.goto('');
  const ids = await idsOf(page, 4);
  // As many entries as the week has items, but with a duplicate and a retired id.
  const stale = [ids[0], ids[0], ...ids.slice(1, -1), 'gone'];
  expect(stale).toHaveLength(ids.length + 1);
  await page.evaluate((v) => localStorage.setItem('fsp:done:week-04', JSON.stringify(v)), stale);
  await page.reload();
  await expect(node(page, 4)).not.toHaveAttribute('data-complete', '');
  await page.evaluate((v) => localStorage.setItem('fsp:done:week-04', JSON.stringify(v)), ids);
  await page.reload();
  await expect(node(page, 4)).toHaveAttribute('data-complete', '');
});

test('a page restored from the back/forward cache re-reads progress', async ({ page }) => {
  await page.goto('');
  await expect(node(page, 4)).not.toHaveAttribute('data-complete', '');
  await page.evaluate((ids) => {
    localStorage.setItem('fsp:done:week-04', JSON.stringify(ids));
    dispatchEvent(new PageTransitionEvent('pageshow', { persisted: true }));
  }, await idsOf(page, 4));
  await expect(node(page, 4)).toHaveAttribute('data-complete', '');
});

test('a week ticked or cleared in another tab updates the open timeline', async ({ context }) => {
  const home = await context.newPage();
  await home.goto('');
  await expect(node(home, 4)).not.toHaveAttribute('data-complete', '');

  const week = await context.newPage();
  await week.goto(WEEK);
  await tickAll(week);
  await expect(node(home, 4)).toHaveAttribute('data-complete', '');

  await week.evaluate(() => localStorage.clear());
  await expect(node(home, 4)).not.toHaveAttribute('data-complete', '');
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
    const n = await tickAll(page);
    for (let i = 0; i < n; i++) await expect(boxes(page).nth(i)).toBeChecked();
    await boxes(page).nth(1).uncheck();
    await expect(boxes(page).nth(1)).not.toBeChecked();

    await page.goto('');
    await expect(page.locator('.week-timeline li[data-week]')).toHaveCount(19);
    expect(errors).toEqual([]);
  });
});
