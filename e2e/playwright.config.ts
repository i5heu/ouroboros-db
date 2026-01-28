import { defineConfig, devices } from '@playwright/test';

/**
 * Playwright configuration for OuroborosDB dashboard E2E tests.
 *
 * The DASHBOARD_URL environment variable should be set by the Go test harness
 * to point to the primary node's dashboard.
 */
export default defineConfig({
  testDir: './tests',
  fullyParallel: false, // Run tests sequentially for predictable state
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: 1, // Single worker for E2E tests
  reporter: [
    ['html', { outputFolder: 'playwright-report' }],
    ['list'],
  ],
  timeout: 60000, // 60 second timeout per test
  expect: {
    timeout: 10000, // 10 second timeout for assertions
  },
  use: {
    baseURL: process.env.DASHBOARD_URL || 'http://localhost:8420',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  // Output directory for test artifacts
  outputDir: 'test-results',
});
