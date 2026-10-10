import { test, expect, type Page } from '@playwright/test';

const WEEK = 'weeks/04-networking/';
const TRACK = 'tracks/foundations/networking/';

const setTheme = (page: Page, theme: 'light' | 'dark') =>
  page.evaluate((t) => {
    document.documentElement.dataset.theme = t;
  }, theme);

test('week page: header, backlinks and Read with links resolve', async ({ page }) => {
  await page.goto(WEEK);
  await expect(page.locator('.week-header')).toContainText('5–8 h');
  await page.locator('.week-header a').click();
  await expect(page).toHaveURL(/weeks\/03-linux-processes\/$/);

  await page.goto(WEEK);
  await page.locator('.read-with a', { hasText: 'DNS' }).click();
  await expect(page).toHaveURL(/tracks\/foundations\/dns\/$/);
  await expect(page.locator('.used-in-weeks')).toContainText('Used in weeks');
  await page.locator('.used-in-weeks a', { hasText: '4' }).click();
  await expect(page).toHaveURL(/weeks\/04-networking\/$/);
});

test('track page: Used in weeks links go to week pages', async ({ page }) => {
  await page.goto(TRACK);
  await expect(page.locator('.used-in-weeks a')).toHaveText(['4', '12']);
});

test('callouts follow the light/dark toggle', async ({ page }) => {
  await page.goto(WEEK);
  const bg = () => page.locator('.callout-observe').evaluate((el) => getComputedStyle(el).backgroundColor);
  await setTheme(page, 'light');
  const light = await bg();
  await setTheme(page, 'dark');
  const dark = await bg();
  expect(light).not.toBe(dark);
  expect(light).not.toBe('rgba(0, 0, 0, 0)');
});

test('side lab is collapsed and opens on click', async ({ page }) => {
  await page.goto(WEEK);
  const lab = page.locator('details.side-lab');
  await expect(lab).not.toHaveAttribute('open', '');
  await lab.locator('summary').click();
  await expect(lab).toHaveAttribute('open', '');
});
