import { defineConfig } from '@playwright/test'

// The extension is loaded into a persistent Chromium context inside each spec (see e2e/support.ts).
// Build first: pnpm --filter @rea/extension build. Unless REA_E2E_API_URL points at an existing
// server, a throw-away API with a fresh database is started (deploy/scripts/e2e-api.sh).
const port = Number(process.env['REA_E2E_API_PORT'] ?? 18105)
if (!process.env['REA_E2E_API_URL']) process.env['REA_E2E_API_URL'] = `http://127.0.0.1:${port}`
const external = process.env['REA_E2E_API_URL'] !== `http://127.0.0.1:${port}` || process.env['REA_E2E_EXTERNAL_API'] === '1'

export default defineConfig({
  testDir: 'e2e',
  timeout: 120_000,
  expect: { timeout: 15_000 },
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [['list']],
  outputDir: 'test-results',
  webServer: external
    ? undefined
    : {
        command: `bash ../../deploy/scripts/e2e-api.sh ${port} ${process.env['REA_E2E_DB'] ?? 'rea_ext_e2e'}`,
        url: `http://127.0.0.1:${port}/readyz`,
        reuseExistingServer: false,
        timeout: 120_000,
        stdout: 'pipe',
        stderr: 'pipe',
      },
})
