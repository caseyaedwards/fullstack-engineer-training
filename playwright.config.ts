import { defineConfig } from '@playwright/test';

const base = (process.env.BASE_PATH ?? '/').replace(/\/?$/, '/');
const port = Number(process.env.E2E_PORT ?? 4321);
const executablePath = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE;

export default defineConfig({
  testDir: 'tests/e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  reporter: 'list',
  use: {
    // Trailing slash so page.goto('dev/diagrams/') resolves under the base.
    baseURL: `http://localhost:${port}${base}`,
    ...(executablePath ? { launchOptions: { executablePath } } : {}),
  },
  webServer: {
    command: `npm run preview -- --ignore-lock --port ${port}`,
    url: `http://localhost:${port}${base}`,
    reuseExistingServer: false,
    timeout: 60_000,
  },
});
