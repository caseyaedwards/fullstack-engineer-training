import { test, expect, type Page } from '@playwright/test';

const PAGE = 'dev/diagrams/';
const flow = (page: Page) => page.locator('figure.dg#dns-lookup');
const caption = (page: Page) => flow(page).locator('.dg-nav [aria-live]');

// Step data-hl lists in the fixture, in order.
const STEPS = [
  ['browser', 'resolver', 'browser->resolver'],
  ['resolver', 'root', 'resolver->root'],
  ['resolver', 'tld', 'resolver->tld'],
  ['browser', 'server', 'browser->server'],
];

async function classes(page: Page) {
  return page.evaluate(() => {
    const out: Record<string, string> = {};
    document.querySelectorAll('figure#dns-lookup [data-k]').forEach((el) => {
      const c = el.classList.contains('dg-hl') ? 'hl' : el.classList.contains('dg-dim') ? 'dim' : 'none';
      out[el.getAttribute('data-k')!] = c;
    });
    return out;
  });
}

test('walkthrough: steps highlight, dim, and navigate', async ({ page }) => {
  await page.goto(PAGE);
  await expect(flow(page).locator('ol.dg-steps')).toBeHidden();
  await expect(caption(page)).toContainText('1/4');
  await expect(caption(page)).toContainText('The browser asks its resolver');

  const prev = flow(page).getByRole('button', { name: 'Prev' });
  const next = flow(page).getByRole('button', { name: 'Next' });
  await expect(prev).toBeDisabled();
  await expect(next).toBeEnabled();

  const check = async (step: number) => {
    const c = await classes(page);
    for (const [k, v] of Object.entries(c)) {
      expect(v, `step ${step + 1} key ${k}`).toBe(STEPS[step].includes(k) ? 'hl' : 'dim');
    }
    for (const k of STEPS[step]) expect(c[k]).toBe('hl');
  };
  await check(0);

  await next.click();
  await expect(caption(page)).toContainText('2/4');
  await check(1);
  await expect(prev).toBeEnabled();

  await prev.click();
  await expect(caption(page)).toContainText('1/4');
  await expect(prev).toBeDisabled();

  await next.focus();
  await page.keyboard.press('ArrowRight');
  await expect(caption(page)).toContainText('2/4');
  await page.keyboard.press('ArrowLeft');
  await expect(caption(page)).toContainText('1/4');

  for (let i = 0; i < 3; i++) await next.click();
  await expect(caption(page)).toContainText('4/4');
  await expect(next).toBeDisabled();
  await check(3);

  // Focus moves off the now-disabled Next so arrow keys keep working.
  await expect(prev).toBeFocused();
  await page.keyboard.press('ArrowLeft');
  await expect(caption(page)).toContainText('3/4');

  await expect(flow(page).getByRole('group', { name: 'Diagram steps' })).toBeVisible();
});

const STEP_MS = 2500;

test.describe('walkthrough: Play', () => {
  test.beforeEach(async ({ page }) => {
    await page.clock.install();
    await page.goto(PAGE);
  });
  const play = (page: Page) => flow(page).getByRole('button', { name: 'Play' });
  const pause = (page: Page) => flow(page).getByRole('button', { name: 'Pause' });

  test('plays through every step, stops on the last, then reads Play again', async ({ page }) => {
    await expect(caption(page)).toContainText('1/4');
    await play(page).click();
    await expect(pause(page)).toBeVisible();
    await expect(caption(page)).toContainText('1/4');
    for (const n of [2, 3, 4]) {
      await page.clock.runFor(STEP_MS);
      await expect(caption(page)).toContainText(`${n}/4`);
      expect((await classes(page))[STEPS[n - 1][0]]).toBe('hl');
    }
    await expect(play(page)).toBeVisible();
    await page.clock.runFor(STEP_MS * 3);
    await expect(caption(page)).toContainText('4/4');
    await expect(caption(page)).toHaveAttribute('aria-live', 'polite');
  });

  test('starts from the current step, and from step 1 when on the last', async ({ page }) => {
    await flow(page).getByRole('button', { name: 'Next' }).click();
    await play(page).click();
    await expect(caption(page)).toContainText('2/4');
    await page.clock.runFor(STEP_MS);
    await expect(caption(page)).toContainText('3/4');
    await pause(page).click();
    await flow(page).getByRole('button', { name: 'Next' }).click();
    await expect(caption(page)).toContainText('4/4');
    await play(page).click();
    await expect(caption(page)).toContainText('1/4');
    await expect(pause(page)).toBeVisible();
  });

  test('Pause stops on the current step', async ({ page }) => {
    await play(page).click();
    await page.clock.runFor(STEP_MS);
    await expect(caption(page)).toContainText('2/4');
    await pause(page).click();
    await expect(play(page)).toBeVisible();
    await page.clock.runFor(STEP_MS * 4);
    await expect(caption(page)).toContainText('2/4');
  });

  test('Next, Prev and arrow keys during playback stop it', async ({ page }) => {
    const nextBtn = flow(page).getByRole('button', { name: 'Next' });
    await play(page).click();
    await page.clock.runFor(STEP_MS);
    await nextBtn.click();
    await expect(caption(page)).toContainText('3/4');
    await expect(play(page)).toBeVisible();
    await page.clock.runFor(STEP_MS * 4);
    await expect(caption(page)).toContainText('3/4');

    await play(page).click();
    await flow(page).getByRole('button', { name: 'Prev' }).click();
    await expect(caption(page)).toContainText('2/4');
    await expect(play(page)).toBeVisible();
    await page.clock.runFor(STEP_MS * 4);
    await expect(caption(page)).toContainText('2/4');

    await play(page).click();
    await nextBtn.focus();
    await page.keyboard.press('ArrowRight');
    await expect(caption(page)).toContainText('3/4');
    await expect(play(page)).toBeVisible();
    await page.clock.runFor(STEP_MS * 4);
    await expect(caption(page)).toContainText('3/4');
  });

  test('playback does not depend on prefers-reduced-motion', async ({ page }) => {
    await page.emulateMedia({ reducedMotion: 'reduce' });
    await play(page).click();
    await page.clock.runFor(STEP_MS * 3);
    await expect(caption(page)).toContainText('4/4');
  });
});

test('walkthrough: nav sits between the steps and the figcaption', async ({ page }) => {
  await page.goto(PAGE);
  const order = await flow(page).evaluate((f) => [...f.children].map((c) => c.className || c.tagName.toLowerCase()));
  expect(order).toEqual(['dg-svg', 'dg-steps', 'dg-nav', 'figcaption']);
});

test.describe('without JavaScript', () => {
  test.use({ javaScriptEnabled: false });
  test('steps list stays visible and nothing is dimmed', async ({ page }) => {
    await page.goto(PAGE);
    await expect(flow(page).locator('ol.dg-steps')).toBeVisible();
    await expect(flow(page).locator('.dg-nav')).toHaveCount(0);
    expect(await page.locator('.dg-dim').count()).toBe(0);
  });
});

function luminance(rgb: string) {
  const [r, g, b] = rgb.match(/[\d.]+/g)!.slice(0, 3).map((v) => {
    const c = Number(v) / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

async function shapeFill(browser: import('@playwright/test').Browser, baseURL: string, stored: 'light' | 'dark', scheme: 'light' | 'dark') {
  const ctx = await browser.newContext({ colorScheme: scheme, baseURL });
  const page = await ctx.newPage();
  await page.addInitScript((t) => localStorage.setItem('starlight-theme', t), stored);
  await page.goto(PAGE);
  const info = await page.evaluate(() => {
    const el = document.querySelector('figure#dns-lookup [data-k="browser"] :is(rect, path)')!;
    return { fill: getComputedStyle(el).fill, theme: document.documentElement.dataset.theme };
  });
  await ctx.close();
  return info;
}

test('theme: the stored Starlight theme, not the OS scheme, drives diagram fills', async ({ browser, baseURL }) => {
  const dark = await shapeFill(browser, baseURL!, 'dark', 'light');
  const light = await shapeFill(browser, baseURL!, 'light', 'dark');
  expect(dark.theme).toBe('dark');
  expect(light.theme).toBe('light');
  expect(dark.fill).toMatch(/^rgb/);
  expect(light.fill).toMatch(/^rgb/);
  expect(dark.fill).not.toBe(light.fill);
  expect(luminance(dark.fill)).toBeLessThan(luminance(light.fill));
});
