import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { expect, test } from '@playwright/test'
import { apiGet, signIn, watchForErrors } from './helpers'

const fixture = (name: string) => fileURLToPath(new URL(`../../../testdata/fixtures/${name}`, import.meta.url))

test.beforeEach(async ({ page }) => signIn(page))

test('analysis: OpenAPI source → findings → deterministic report', async ({ page }, testInfo) => {
  const done = watchForErrors(page, testInfo)
  // Create the project and source through the API (the wizard itself is covered by component
  // tests), then use the UI for everything that follows.
  const csrf = (await (await page.request.get('/api/v1/auth/me')).json()).csrf_token as string
  const ws = await apiGet<{ items: { id: string }[] }>(page, '/workspaces')
  const project = await (
    await page.request.post('/api/v1/analysis/projects', {
      headers: { 'X-CSRF-Token': csrf },
      data: {
        name: 'Inventory API', description: '', objective: 'Document the API', workspace_id: ws.items[0]!.id, analysis_types: ['api'],
        status: 'active', owner_id: null, authorization_confirmed: true, authorization_basis: 'owned', authorization_note: 'E2E fixture',
      },
    })
  ).json()
  const up = await page.request.post(`/api/v1/analysis/projects/${project.id}/sources/upload`, {
    headers: { 'X-CSRF-Token': csrf },
    multipart: { kind: 'openapi', file: { name: 'openapi-inventory.yaml', mimeType: 'application/yaml', buffer: readFileSync(fixture('openapi-inventory.yaml')) } },
  })
  expect(up.status(), await up.text()).toBe(201)

  await page.goto(`/analysis/${project.id}`)
  await expect(page.getByRole('heading', { name: 'Inventory API' })).toBeVisible()
  await expect.poll(async () => (await apiGet<{ items: { status: string }[] }>(page, `/analysis/projects/${project.id}/runs`)).items[0]?.status, { timeout: 30_000 }).toBe('succeeded')

  await page.getByRole('tab', { name: /Findings/ }).click()
  const row = page.getByRole('row', { name: /API: Warehouse Inventory API/ })
  await expect(row).toBeVisible()
  await expect(row).toContainText(/verified/i)

  await page.getByRole('tab', { name: /Reports/ }).click()
  await page.getByLabel('Report type').selectOption('api_inventory')
  await page.getByRole('button', { name: /Generate/ }).first().click()
  await expect(page.getByText('Deterministic report generated from findings')).toBeVisible()
  await expect(page.getByText(/No AI was used/)).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Endpoints' })).toBeVisible()
  done()
})
