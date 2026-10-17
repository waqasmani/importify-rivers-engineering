// A marketplace definition shipped with REA, end to end: the API delivers it through
// /extension/config, the real extension runs it on a page served under the marketplace's own
// hostname and the product is imported into the review queue.
//
// The marketplace is never contacted: Chromium runs without a proxy and cannot resolve any host
// but the local API, and Playwright answers the page request with the synthetic fixture
// (testdata/fixtures/marketplaces/aliexpress.html) and aborts every other request to the host.
import { expect, test, type Page, type Route } from '@playwright/test'
import { EXTENSION_ID, launchExtension, marketplaceFixture, OFFLINE_ARGS, ReaSession, testCopyWithFixtureAccess, type Launched } from './support'

const API = process.env['REA_E2E_API_URL'] ?? ''
const OWNER = {
  email: process.env['REA_E2E_EMAIL'] ?? 'owner@e2e.example',
  password: process.env['REA_E2E_PASSWORD'] ?? 'Northwind-Flask-2026!',
  org: 'Northwind Sourcing',
  name: 'E2E Owner',
}
const PAGE_URL = 'https://www.aliexpress.com/item/1005001234567890.html'
const TITLE = 'Folding LED Desk Lamp, 3 Colour Modes'

test.describe.configure({ mode: 'serial' })
test.skip(!API, 'Set REA_E2E_API_URL to the REA API under test to run the end-to-end flow.')

let ext: Launched
let copy: { dir: string; cleanup: () => Promise<void> }
let rea: ReaSession
let panel: Page
let deviceId = ''
const served: string[] = []

test.beforeAll(async () => {
  rea = new ReaSession(API)
  await rea.signIn(OWNER, process.env['REA_E2E_SETUP_TOKEN'] ?? 'dev-setup-token')
  copy = await testCopyWithFixtureAccess(['https://www.aliexpress.com/*'])
  ext = await launchExtension(copy.dir, OFFLINE_ARGS)
  const html = await marketplaceFixture('aliexpress')
  await ext.context.route(/^https?:\/\/([^/]+\.)?(aliexpress\.(com|us)|alicdn\.example)\//, async (route: Route) => {
    const url = route.request().url()
    served.push(url)
    if (url === PAGE_URL) await route.fulfill({ status: 200, contentType: 'text/html; charset=utf-8', body: html })
    else await route.abort()
  })
})

test.afterAll(async () => {
  await ext?.close()
  await copy?.cleanup()
})

test('the server ships the definition and delivers it to a paired extension', async () => {
  const adapters = await rea.request<{ items: { key: string; kind: string; shipped: boolean; enabled: boolean; verification: { status: string } }[] }>('GET', '/adapters')
  expect(adapters.body.items.find((a) => a.key === 'aliexpress')).toMatchObject({ kind: 'rules', shipped: true, enabled: true, verification: { status: 'synthetic_fixture' } })

  const code = (await rea.request<{ code: string }>('POST', '/extension/pairing-codes', {})).body.code
  // Navigate from Playwright so that the route answers the request.
  const site = await ext.context.newPage()
  await site.goto(PAGE_URL)
  await expect(site.locator('h1')).toHaveText(TITLE)
  const tabId = await ext.worker.evaluate(async (u) => (await chrome.tabs.query({ url: u }))[0]?.id ?? -1, PAGE_URL)
  expect(tabId).toBeGreaterThan(0)
  panel = await ext.context.newPage()
  await panel.goto(`chrome-extension://${EXTENSION_ID}/sidepanel.html?tabId=${tabId}`)
  await panel.getByLabel('Server address').fill(API)
  await panel.getByLabel('Pairing code').fill(code)
  await panel.getByLabel('Device name').fill('Playwright marketplace')
  await panel.getByTestId('pair').click()
  await expect(panel.getByTestId('connection-summary')).toHaveText(`${OWNER.org} · ${OWNER.email}`)
  const status = await panel.evaluate(() => chrome.runtime.sendMessage({ type: 'status.get', refresh: false }))
  deviceId = status.data.deviceId
  expect(served).toEqual([PAGE_URL])
})

test('reads the marketplace page with the shipped definition', async () => {
  await panel.getByTestId('nav-capture').click()
  await panel.getByTestId('extract').click()
  await expect(panel.getByTestId('product-title')).toHaveText(TITLE)
  const summary = panel.getByTestId('extraction-summary')
  await expect(summary).toContainText('AliExpress')
  await expect(summary).toContainText(/aliexpress/)
  await expect(panel.getByTestId('page-type')).toHaveValue('product')
  await panel.getByTestId('save-capture').click()
  await expect(panel.getByTestId('capture-saved')).toHaveText('Saved capture of aliexpress.com.')
})

test('imports the product with its variants into the review queue', async () => {
  await panel.getByTestId('nav-import').click()
  await expect(panel.getByLabel('Title')).toHaveValue(TITLE)
  await expect(panel.getByLabel('Price')).toHaveValue('18.4 USD')
  await expect(panel.getByLabel('Currency')).toHaveValue('USD')
  await panel.getByTestId('import-check').click()
  const check = panel.getByTestId('check-result')
  await expect(check).toContainText('18.4')
  let expected: 'imported' | 'updated' = 'imported'
  await expect(check).toContainText(/No duplicates found in REA\.|Definite duplicate found/)
  if ((await check.textContent())?.includes('Definite duplicate found')) {
    expected = 'updated'
    await panel.getByLabel('Update an existing product').check()
  }
  await panel.getByTestId('import-submit').click()
  await expect(panel.getByTestId('import-outcome')).toHaveText(
    expected === 'imported' ? "Imported as a draft product. It is waiting in REA's review queue." : 'The existing product was refreshed with this capture.',
  )

  const captures = await rea.request<{ items: { id: string; url: string; adapter_key: string; page_type: string }[] }>('GET', `/captures?device_id=${deviceId}`)
  const capture = captures.body.items.find((c) => c.url === PAGE_URL)!
  expect(capture).toMatchObject({ adapter_key: 'aliexpress', page_type: 'product' })
  const detail = await rea.request<{ capture: { extracted: { product: { evidence: Record<string, string> }; metadata: Record<string, { value: string }> } } }>('GET', `/captures/${capture.id}`)
  expect(detail.body.capture.extracted.product.evidence['title']).toBe('aliexpress: script "window.runParams" → data.productInfoComponent.subject (rule title[0])')
  expect(detail.body.capture.extracted.metadata['rating_count']?.value).toBe('312')
  expect(JSON.stringify(detail.body)).not.toMatch(/should-not-leak/)

  const imports = await rea.request<{ items: { status: string; product_id: string | null; captured_page_id: string | null }[] }>('GET', '/imports?source=extension')
  const imp = imports.body.items.find((i) => i.captured_page_id === capture.id)!
  expect(imp.status).toBe(expected)
  const product = await rea.request<{ status: string; review_status: string; price: string; price_currency: string; brand: string; source_domain: string; variants: { sku: string; options: Record<string, string> }[] }>('GET', `/products/${imp.product_id}`)
  expect(product.body).toMatchObject({ status: 'draft', review_status: 'pending', price_currency: 'USD', brand: 'LUMOFOLD', source_domain: 'aliexpress.com' })
  expect(Number(product.body.price)).toBe(18.4)
  expect(product.body.variants.map((v) => v.sku).sort()).toEqual(['12000031001', '12000031002', '12000031003'])
  expect(product.body.variants.find((v) => v.sku === '12000031001')?.options).toEqual({ Color: 'Black', 'Ships From': 'CHINA' })
})

test('disconnects', async () => {
  await panel.getByTestId('nav-connection').click()
  await panel.getByTestId('disconnect').click()
  await panel.getByTestId('confirm-disconnect').click()
  await expect(panel.getByRole('heading', { name: 'Pair with your REA server' })).toBeVisible()
  // The only marketplace request was the routed page; the side panel's image previews asked for
  // the fixture's invented CDN images, which the route aborted.
  expect(served.filter((u) => /(^|\.)aliexpress\.(com|us)$/.test(new URL(u).hostname))).toEqual([PAGE_URL])
  for (const u of served.filter((x) => x !== PAGE_URL)) expect(new URL(u).hostname).toBe('ae01.alicdn.example')
})
