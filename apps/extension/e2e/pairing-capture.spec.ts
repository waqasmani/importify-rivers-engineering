// Real end-to-end run against a REA API: pair from the side panel page, pick an element, capture a
// fixture page with a sanitized snapshot, import it into the review queue, verify the records
// through the API and disconnect. Requires REA_E2E_API_URL (for example http://127.0.0.1:18105).
import { expect, test, type Page } from '@playwright/test'
import { EXTENSION_ID, FIXTURE_ORIGIN, launchExtension, openTab, ReaSession, serveFixtures, testCopyWithFixtureAccess, type Launched } from './support'

const API = process.env['REA_E2E_API_URL'] ?? ''
const OWNER = {
  email: process.env['REA_E2E_EMAIL'] ?? 'owner@e2e.example',
  password: process.env['REA_E2E_PASSWORD'] ?? 'Northwind-Flask-2026!',
  org: 'Northwind Sourcing',
  name: 'E2E Owner',
}

test.describe.configure({ mode: 'serial' })
test.skip(!API, 'Set REA_E2E_API_URL to the REA API under test to run the end-to-end flow.')

let ext: Launched
let fixtures: { close: () => Promise<void> }
let copy: { dir: string; cleanup: () => Promise<void> }
let rea: ReaSession
let panel: Page
let site: Page
let tabId: number
let deviceId: string
let expectedStatus: 'imported' | 'updated' = 'imported'
let captureId = ''

interface Capture { id: string; url: string; adapter_key: string; page_type: string; device_id: string | null; has_snapshot: boolean; selection_count: number }

test.beforeAll(async () => {
  rea = new ReaSession(API)
  await rea.signIn(OWNER, process.env['REA_E2E_SETUP_TOKEN'] ?? 'dev-setup-token')
  const put = await rea.request('PUT', '/settings/capture', { store_html_snapshots: true, max_snapshot_kb: 512, retention_days: 0 })
  expect(put.status).toBe(200)
  // An external (non-local) provider exercises the explicit data-sharing confirmation.
  const providers = await rea.request<{ items: { name: string }[] }>('GET', '/ai/providers')
  if (!providers.body.items.some((p) => p.name === 'E2E external provider')) {
    const created = await rea.request('POST', '/ai/providers', {
      name: 'E2E external provider', kind: 'openai_compatible', base_url: 'https://ai.e2e.invalid/v1', model: 'e2e-model', api_key: 'sk-e2e-placeholder',
      is_default: true, enabled: true, allow_external_data: true, supports_vision: false, temperature: '0.2', max_output_tokens: 512, timeout_seconds: 5,
    })
    expect(created.status).toBe(201)
  }
  fixtures = await serveFixtures()
  copy = await testCopyWithFixtureAccess()
  ext = await launchExtension(copy.dir)
})

test.afterAll(async () => {
  await ext?.close()
  await copy?.cleanup()
  await fixtures?.close()
})

test('pairs from the side panel page', async () => {
  expect(ext.extensionId).toBe(EXTENSION_ID)
  let code = process.env['REA_E2E_PAIRING_CODE'] ?? ''
  if (!code) {
    const res = await rea.request<{ code: string }>('POST', '/extension/pairing-codes', {})
    expect(res.status).toBe(201)
    code = res.body.code
  }
  tabId = await openTab(ext.worker, `${FIXTURE_ORIGIN}/product-jsonld.html`)
  site = ext.context.pages().find((p) => p.url().startsWith(FIXTURE_ORIGIN))!
  panel = await ext.context.newPage()
  await panel.goto(`chrome-extension://${EXTENSION_ID}/sidepanel.html?tabId=${tabId}`)
  await panel.getByLabel('Server address').fill(API)
  await panel.getByLabel('Pairing code').fill(code)
  await panel.getByLabel('Device name').fill('Playwright Chromium')
  await panel.getByTestId('pair').click()
  await expect(panel.getByTestId('connection-summary')).toHaveText(`${OWNER.org} · ${OWNER.email}`)

  // Tokens live only in extension storage: the access token in session storage, nothing in local storage.
  const stored = await ext.worker.evaluate(async () => ({
    local: await chrome.storage.local.get(null),
    sessionKeys: Object.keys(await chrome.storage.session.get(null)),
  }))
  expect(stored.local).toEqual({})
  expect(stored.sessionKeys).toContain('access')
  const status = await panel.evaluate(() => chrome.runtime.sendMessage({ type: 'status.get', refresh: false }))
  deviceId = status.data.deviceId
  const devices = await rea.request<{ items: { id: string; name: string; browser: string; extension_version: string; revoked_at: string | null }[] }>('GET', '/extension/devices')
  expect(devices.body.items.find((d) => d.id === deviceId)).toMatchObject({ name: 'Playwright Chromium', extension_version: '1.0.0', revoked_at: null })
})

test('turns on snapshots within the server limit', async () => {
  await panel.getByTestId('nav-settings').click()
  await expect(panel.getByText('Sites you allowed (1)')).toBeVisible()
  await panel.getByLabel('Include a sanitized HTML snapshot when I save a capture').check()
  await panel.getByLabel(/Size limit/).fill('256')
  await panel.getByRole('button', { name: 'Save snapshot preference' }).click()
  await expect(panel.getByText('Saved.')).toBeVisible()
})

test('reads the page, picks the specification table and saves the capture', async () => {
  await panel.getByTestId('nav-capture').click()
  await panel.getByTestId('extract').click()
  await expect(panel.getByTestId('product-title')).toHaveText('Trailblazer 2L Insulated Flask')
  await expect(panel.getByTestId('page-type')).toHaveValue('product')
  await expect(panel.getByTestId('extraction-summary')).toContainText('Sanitized snapshot')

  await panel.getByRole('button', { name: 'Specifications table' }).click()
  await expect(panel.getByText('Picking Specifications table on the page')).toBeVisible()
  await site.locator('table.specs td').first().click()
  await expect(panel.getByTestId('selections')).toContainText('3 rows')
  await site.keyboard.press('Escape')
  await expect(panel.getByText('Picking Specifications table on the page')).toBeHidden()
  expect(await site.locator('[data-rea-picker]').count()).toBe(0)

  await panel.getByTestId('save-capture').click()
  await expect(panel.getByTestId('capture-saved')).toHaveText('Saved capture of 127.0.0.1.')
})

test('checks and imports the product into the review queue', async () => {
  await panel.getByTestId('nav-import').click()
  await expect(panel.getByLabel('Title')).toHaveValue('Trailblazer 2L Insulated Flask')
  await expect(panel.getByLabel('Price')).toHaveValue('39.95 EUR')
  await expect(panel.getByLabel('Currency')).toHaveValue('EUR')
  await panel.getByTestId('import-check').click()
  const check = panel.getByTestId('check-result')
  await expect(check).toContainText('39.95')
  await expect(check).toContainText(/No duplicates found in REA\.|Definite duplicate found/)
  if ((await check.textContent())?.includes('Definite duplicate found')) {
    // A previous run against this server imported the same product: refresh it instead.
    expectedStatus = 'updated'
    await panel.getByLabel('Update an existing product').check()
  } else {
    await expect(panel.getByLabel('Create a new draft product')).toBeChecked()
  }
  await panel.getByTestId('import-submit').click()
  await expect(panel.getByTestId('import-outcome')).toHaveText(
    expectedStatus === 'imported' ? "Imported as a draft product. It is waiting in REA's review queue." : 'The existing product was refreshed with this capture.',
  )
})

test('the capture and import exist on the server', async () => {
  const device = { id: deviceId }
  const captures = await rea.request<{ items: Capture[] }>('GET', `/captures?device_id=${device.id}`)
  expect(captures.status).toBe(200)
  const capture = captures.body.items.find((c) => c.url === `${FIXTURE_ORIGIN}/product-jsonld.html`)!
  expect(capture).toBeTruthy()
  captureId = capture.id
  expect(capture).toMatchObject({ adapter_key: 'json-ld', page_type: 'product', device_id: device.id, has_snapshot: true, selection_count: 1 })

  const detail = await rea.request<{ capture: { selections: { kind: string; label: string; rows: string[][] }[]; extracted: { product: { title: string } } } }>('GET', `/captures/${capture.id}`)
  expect(detail.body.capture.selections[0]).toMatchObject({ kind: 'table', label: 'specifications', rows: [['Capacity', '2 L'], ['Material', '18/8 stainless steel'], ['Weight', '820 g']] })
  expect(detail.body.capture.extracted.product.title).toBe('Trailblazer 2L Insulated Flask')
  const snapshot = await fetch(`${API}/api/v1/captures/${capture.id}/snapshot`, { headers: { Cookie: rea.cookie } }).then((r) => r.text())
  expect(snapshot).toContain('Trailblazer 2L Insulated Flask')
  expect(snapshot).not.toMatch(/<script|type="hidden"|googletagmanager/i)

  const imports = await rea.request<{ items: { id: string; status: string; source: string; product_id: string | null; product_title: string | null; captured_page_id: string | null; device_id: string | null }[] }>('GET', '/imports?source=extension')
  const imp = imports.body.items.find((i) => i.captured_page_id === capture.id)!
  expect(imp).toMatchObject({ status: expectedStatus, source: 'extension', product_title: 'Trailblazer 2L Insulated Flask', device_id: device.id })
  const product = await rea.request<{ status: string; review_status: string; price: string; price_currency: string; sku: string; gtin: string; source_domain: string; captured_page_id: string }>('GET', `/products/${imp.product_id}`)
  expect(product.status).toBe(200)
  expect(product.body).toMatchObject({ status: 'draft', review_status: 'pending', price_currency: 'EUR', sku: 'NW-TB-2L', gtin: '4006381333931', source_domain: '127.0.0.1', captured_page_id: capture.id })
  if (expectedStatus === 'updated') expect(imp.product_id).toBeTruthy()
  expect(Number(product.body.price)).toBe(39.95)
})

test('creates an analysis project from the capture', async () => {
  await panel.getByTestId('nav-analysis').click()
  await panel.getByLabel('Project name').fill('E2E analysis of 127.0.0.1')
  await panel.getByLabel('I confirm I may analyze this material').check()
  await panel.getByRole('button', { name: 'Create analysis project' }).click()
  await expect(panel.getByText('Project created and analysis queued.')).toBeVisible()
  const href = await panel.getByRole('link', { name: 'Open project' }).getAttribute('href')
  const projectId = href!.split('/').pop()!
  const sources = await rea.request<{ items: { kind: string; captured_page_id: string | null }[] }>('GET', `/analysis/projects/${projectId}/sources`)
  expect(sources.body.items).toEqual([expect.objectContaining({ kind: 'captured_page', captured_page_id: captureId })])
})

test('asks for explicit confirmation before sending the capture to an external AI provider', async () => {
  await panel.getByRole('button', { name: 'Summarize with AI' }).click()
  const confirm = panel.getByText('This sends data to an external AI provider.')
  await expect(confirm).toBeVisible()
  await expect(panel.getByText('ai.e2e.invalid', { exact: true })).toBeVisible()
  await expect(panel.getByText('captured page content and selections', { exact: true })).toBeVisible()
  await panel.getByTestId('ai-confirm').click()
  await expect(panel.getByText(/^Status: (queued|running|failed|succeeded)/)).toBeVisible()
  const jobs = await rea.request<{ items: { id: string; tool: string; entity_id: string }[] }>('GET', `/ai/jobs?entity_id=${captureId}`)
  expect(jobs.body.items).toEqual([expect.objectContaining({ tool: 'summarize', entity_id: captureId })])
  const job = await rea.request<{ job: { external_data_confirmed: boolean; provider_name: string } }>('GET', `/ai/jobs/${jobs.body.items[0]!.id}`)
  expect(job.body.job).toMatchObject({ external_data_confirmed: true, provider_name: 'E2E external provider' })
})

test('lists recent captures and searches REA', async () => {
  await panel.getByTestId('nav-recent').click()
  await expect(panel.getByTestId('recent-captures')).toContainText('Trailblazer 2L Insulated Flask')
  await panel.getByTestId('nav-research').click()
  await panel.getByLabel('Search terms').fill('Trailblazer')
  await panel.getByRole('button', { name: 'Search', exact: true }).click()
  await expect(panel.getByRole('link', { name: 'Trailblazer 2L Insulated Flask' }).first()).toBeVisible()
})

test('disconnect revokes the device and wipes local credentials', async () => {
  await panel.getByTestId('nav-connection').click()
  await panel.getByTestId('disconnect').click()
  await panel.getByTestId('confirm-disconnect').click()
  await expect(panel.getByRole('heading', { name: 'Pair with your REA server' })).toBeVisible()
  const devices = await rea.request<{ items: { id: string; revoked_at: string | null }[] }>('GET', '/extension/devices')
  expect(devices.body.items.find((d) => d.id === deviceId)?.revoked_at).toBeTruthy()
  const left = await ext.worker.evaluate(async () => Object.keys(await chrome.storage.session.get(null)))
  expect(left).not.toContain('access')
  const status = await panel.evaluate(() => chrome.runtime.sendMessage({ type: 'status.get', refresh: false }))
  expect(status).toMatchObject({ ok: true, data: { connected: false } })
})

test('a device revoked in REA is signed out on its next request', async () => {
  const res = await rea.request<{ code: string }>('POST', '/extension/pairing-codes', {})
  await panel.getByLabel('Server address').fill(API)
  await panel.getByLabel('Pairing code').fill(res.body.code)
  await panel.getByLabel('Device name').fill('Playwright revoked')
  await panel.getByTestId('pair').click()
  await expect(panel.getByTestId('connection-summary')).toHaveText(`${OWNER.org} · ${OWNER.email}`)
  const status = await panel.evaluate(() => chrome.runtime.sendMessage({ type: 'status.get', refresh: false }))
  const revoked = await rea.request('DELETE', `/extension/devices/${status.data.deviceId}`)
  expect(revoked.status).toBe(204)
  // Expire the cached access token so the next call has to use the (now revoked) refresh token.
  await ext.worker.evaluate(async () => {
    const { access } = (await chrome.storage.session.get('access')) as { access: Record<string, unknown> }
    await chrome.storage.session.set({ access: { ...access, expiresAt: new Date(Date.now() - 1000).toISOString() } })
  })
  await panel.getByTestId('nav-connection').click()
  await panel.getByTitle('Check the connection').click()
  await expect(panel.getByRole('heading', { name: 'Pair with your REA server' })).toBeVisible()
  await expect(panel.getByRole('alert')).toContainText('expired or was revoked')
  const after = await panel.evaluate(() => chrome.runtime.sendMessage({ type: 'status.get', refresh: false }))
  expect(after.data.connected).toBe(false)
})
