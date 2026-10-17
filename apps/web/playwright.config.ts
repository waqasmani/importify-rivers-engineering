import { defineConfig, devices } from '@playwright/test'

// End-to-end tests run the real API (deploy/scripts/e2e-api.sh) against a fresh database and
// drive the built dashboard it serves. Build first: pnpm --filter @rea/web build.
const port = Number(process.env.REA_E2E_PORT ?? 18190)

export default defineConfig({
  testDir: 'e2e',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout: 90_000,
  expect: { timeout: 10_000 },
  reporter: [['list'], ['html', { open: 'never' }]],
  use: {
    baseURL: `http://127.0.0.1:${port}`,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    // Locally: PW_CHROMIUM_PATH=/opt/pw-browsers/chromium. CI installs Playwright's own browser.
    launchOptions: process.env.PW_CHROMIUM_PATH ? { executablePath: process.env.PW_CHROMIUM_PATH } : {},
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: {
    command: `bash ../../deploy/scripts/e2e-api.sh ${port}`,
    url: `http://127.0.0.1:${port}/readyz`,
    reuseExistingServer: false,
    timeout: 120_000,
    stdout: 'pipe',
    stderr: 'pipe',
  },
})
