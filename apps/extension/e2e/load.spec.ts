// Loads the shipped dist/ (no host permissions) and checks the extension identity, the side
// panel UI, message validation and that it cannot read a page without a user grant.
import { expect, test } from '@playwright/test'
import { DIST, EXTENSION_ID, FIXTURE_ORIGIN, launchExtension, openTab, serveFixtures, type Launched } from './support'

let ext: Launched
let fixtures: { close: () => Promise<void> }

test.beforeAll(async () => {
  fixtures = await serveFixtures()
  ext = await launchExtension(DIST)
})

test.afterAll(async () => {
  await ext?.close()
  await fixtures?.close()
})

test('the service worker registers with the fixed extension ID', async () => {
  expect(ext.extensionId).toBe(EXTENSION_ID)
  expect(ext.worker.url()).toBe(`chrome-extension://${EXTENSION_ID}/background.js`)
  const manifest = await ext.worker.evaluate(() => chrome.runtime.getManifest())
  expect(manifest.manifest_version).toBe(3)
  expect(manifest.host_permissions).toBeUndefined()
  expect(manifest.permissions).toEqual(['activeTab', 'scripting', 'storage', 'contextMenus', 'sidePanel'])
  const granted = await ext.worker.evaluate(() => chrome.permissions.getAll())
  expect(granted.origins ?? []).toEqual([])
})

test('the side panel page renders the pairing form', async () => {
  const page = await ext.context.newPage()
  const errors: string[] = []
  page.on('pageerror', (e) => errors.push(e.message))
  page.on('console', (m) => { if (m.type() === 'error') errors.push(m.text()) })
  await page.goto(`chrome-extension://${EXTENSION_ID}/sidepanel.html`)
  await expect(page.getByRole('heading', { name: 'REA Importify' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Pair with your REA server' })).toBeVisible()
  await expect(page.getByTestId('connection-summary')).toHaveText('Not connected')
  await page.getByLabel('Server address').fill('http://rea.example.com')
  await expect(page.getByText('Use https:// for REA servers that are not on this computer.')).toBeVisible()
  await page.getByTestId('nav-settings').click()
  await expect(page.getByRole('heading', { name: 'Site access' })).toBeVisible()
  expect(errors).toEqual([])
  await page.close()
})

test('the popup page renders', async () => {
  const page = await ext.context.newPage()
  await page.goto(`chrome-extension://${EXTENSION_ID}/popup.html`)
  await expect(page.getByRole('heading', { name: 'REA Importify' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Open side panel' })).toBeVisible()
  await page.close()
})

test('the service worker rejects malformed requests', async () => {
  const page = await ext.context.newPage()
  await page.goto(`chrome-extension://${EXTENSION_ID}/sidepanel.html`)
  const res = await page.evaluate(async () => [
    await chrome.runtime.sendMessage({ type: 'api.raw', path: '/users' }),
    await chrome.runtime.sendMessage({ type: 'status.get', refresh: false, token: 'x' }),
    await chrome.runtime.sendMessage({ type: 'status.get', refresh: false }),
  ])
  expect(res[0]).toMatchObject({ ok: false, error: { code: 'invalid_request' } })
  expect(res[1]).toMatchObject({ ok: false, error: { code: 'invalid_request' } })
  expect(res[2]).toMatchObject({ ok: true, data: { connected: false, extensionId: EXTENSION_ID } })
  await page.close()
})

test('web pages cannot reach the extension and it cannot read them without a grant', async () => {
  const tabId = await openTab(ext.worker, `${FIXTURE_ORIGIN}/product-jsonld.html`)
  const site = ext.context.pages().find((p) => p.url().startsWith(FIXTURE_ORIGIN))!
  expect(await site.evaluate(() => typeof (globalThis as { chrome?: { runtime?: { sendMessage?: unknown } } }).chrome?.runtime?.sendMessage)).toBe('undefined')

  const panel = await ext.context.newPage()
  await panel.goto(`chrome-extension://${EXTENSION_ID}/sidepanel.html?tabId=${tabId}`)
  await panel.getByTestId('nav-capture').click()
  await expect(panel.getByText("REA can't read this tab yet.")).toBeVisible()
  await panel.getByTestId('extract').click()
  await expect(panel.getByRole('alert')).toContainText('REA does not have access to this page yet')
  await panel.close()
})
