// region: config
import { defineConfig, devices } from '@playwright/test'

// Set PLAYWRIGHT_CHROMIUM_EXECUTABLE to run a Chromium you already have
// instead of the one that npx playwright install downloads.
const executablePath = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE

export default defineConfig({
  testDir: 'e2e',
  forbidOnly: !!process.env.CI, // a test.only left in fails CI
  use: {
    baseURL: 'http://localhost:5173',
    trace: 'retain-on-failure', // a failed test leaves a trace in test-results/
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'], ...(executablePath ? { launchOptions: { executablePath } } : {}) },
    },
  ],
  // Playwright starts the Vite dev server before the tests and stops it
  // after them. On your laptop, it reuses one that already runs.
  webServer: {
    command: 'npx vite --port 5173 --strictPort',
    url: 'http://localhost:5173',
    reuseExistingServer: !process.env.CI,
  },
})
// endregion: config
