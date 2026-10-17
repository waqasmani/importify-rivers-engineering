import { expect, test, type Page } from '@playwright/test'
import { apiGet, signIn, watchForErrors } from './helpers'

// An integration key submits a customer order; a person confirms the prepared supplier order in
// the dashboard, records the placement made on the supplier's site and the shipment.

async function csrf(page: Page): Promise<string> {
  return (await (await page.request.get('/api/v1/auth/me')).json()).csrf_token as string
}

async function post<T>(page: Page, path: string, data: object): Promise<T> {
  const res = await page.request.post(`/api/v1${path}`, { headers: { 'X-CSRF-Token': await csrf(page) }, data })
  expect(res.ok(), `${path}: ${res.status()} ${await res.text()}`).toBeTruthy()
  return (await res.json()) as T
}

async function noHorizontalScroll(page: Page) {
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
  expect(overflow, 'the page scrolls horizontally').toBeLessThanOrEqual(0)
}

test('an order from an integration key is confirmed, placed and shipped by a person', async ({ page, request }, testInfo) => {
  const done = watchForErrors(page, testInfo)
  await signIn(page)

  // Catalog: a published product of a supplier that is placed by hand.
  const supplier = await post<{ id: string }>(page, '/suppliers', { name: 'Orders E2E Mugs' })
  const product = await post<{ id: string }>(page, '/products', {
    title: 'E2E enamel mug', sku: 'E2E-MUG-1', supplier_id: supplier.id, source_url: 'https://mugs.example.com/p/e2e-enamel',
    cost: '3.10', cost_currency: 'EUR', price: '19.90', price_currency: 'EUR', availability: 'in_stock',
  })
  await post(page, `/products/${product.id}/review`, { review_status: 'approved' })
  const key = await post<{ token: string }>(page, '/integrations/keys', { name: 'E2E storefront', scopes: ['orders.write'] })

  // The storefront sends the order with its key (not the browser session).
  const submitted = await request.post('/api/v1/orders', {
    headers: { Authorization: `Bearer ${key.token}` },
    data: {
      external_id: 'E2E-1001', currency: 'EUR', total: '39.80',
      customer: { name: 'Grace Hopper', email: 'grace@example.org' },
      shipping_address: { name: 'Grace Hopper', line1: '1 Compiler Lane', city: 'Arlington', postal_code: '22201', region: 'VA', country: 'US' },
      lines: [{ sku: 'E2E-MUG-1', quantity: 2, unit_price: '19.90' }],
    },
  })
  expect(submitted.status(), await submitted.text()).toBe(201)
  const order = (await submitted.json()) as { id: string; number: string; status: string }
  expect(order.status).toBe('awaiting_confirmation')
  // The key cannot confirm anything itself.
  const so = (await apiGet<{ supplier_orders: { id: string }[] }>(page, `/orders/${order.id}`)).supplier_orders[0]!
  const keyConfirm = await request.post(`/api/v1/supplier-orders/${so.id}/confirm`, {
    headers: { Authorization: `Bearer ${key.token}` }, data: { content_hash: 'a'.repeat(64), acknowledge_warnings: true },
  })
  expect(keyConfirm.status()).toBe(403)

  // The order shows up in the dashboard.
  await page.goto('/orders')
  await expect(page.getByRole('link', { name: order.number })).toBeVisible()
  await page.getByRole('link', { name: order.number }).click()
  await expect(page.getByRole('heading', { name: `Order ${order.number}` })).toBeVisible()
  await expect(page.getByTestId('order-status')).toHaveText('Awaiting confirmation')
  const card = page.getByTestId(`supplier-order-${so.id}`)
  await expect(card.getByTestId('so-status')).toHaveText('Draft · to confirm')

  // Confirm: the dialog lists what is handed off.
  await card.getByTestId('open-confirm').click()
  const dialog = page.getByTestId('confirm-dialog')
  await expect(dialog.getByTestId('confirm-items')).toContainText('2 × E2E enamel mug')
  await expect(dialog).toContainText('1 Compiler Lane')
  await expect(dialog).toContainText('Manual placement')
  const ack = dialog.getByTestId('confirm-ack')
  if (await ack.isVisible().catch(() => false)) await ack.check()
  await page.getByTestId('confirm-submit').click()
  await expect(card.getByTestId('so-status')).toHaveText('Confirmed · to place')
  await expect(page.getByTestId('order-status')).toHaveText('In progress')

  // Place it on the supplier's site (outside REA) and record it from the placement sheet.
  await card.getByTestId('open-placement').click()
  const sheet = card.getByTestId('placement-sheet')
  await expect(sheet.getByRole('link', { name: 'Open supplier page' })).toHaveAttribute('href', 'https://mugs.example.com/p/e2e-enamel')
  await expect(sheet.getByTestId('ship-to-fields')).toContainText('1 Compiler Lane')
  await sheet.getByLabel('Supplier order number').fill('MUG-77001')
  await sheet.getByLabel('Actual cost').fill('6.10')
  await sheet.getByRole('button', { name: 'Record placement' }).click()
  await expect(card.getByTestId('so-status')).toHaveText('Placed · awaiting tracking')
  await expect(page.getByTestId('order-status')).toHaveText('Placed')

  // Record the shipment.
  await card.getByTestId('open-shipment').click()
  await page.getByLabel('Carrier').fill('UPS')
  await page.getByLabel('Tracking number').fill('1Z999AA10123456784')
  await page.getByLabel('Tracking link').fill('https://www.ups.com/track?tracknum=1Z999AA10123456784')
  await page.getByRole('button', { name: 'Save tracking' }).click()
  await expect(card.getByTestId('so-status')).toHaveText('Shipped')
  await expect(card.getByTestId('so-tracking')).toContainText('1Z999AA10123456784')
  await expect(page.getByTestId('order-status')).toHaveText('Shipped')

  // The storefront sees the status and tracking through its key.
  const seen = await request.get('/api/v1/orders/by-external-id/E2E-1001', { headers: { Authorization: `Bearer ${key.token}` } })
  expect(seen.status()).toBe(200)
  const body = (await seen.json()) as { status: string; supplier_orders: { tracking_number: string }[] }
  expect(body.status).toBe('shipped')
  expect(body.supplier_orders[0]!.tracking_number).toBe('1Z999AA10123456784')

  // Phone width: no horizontal scrolling on the order pages.
  await page.setViewportSize({ width: 390, height: 844 })
  await page.reload()
  await expect(page.getByTestId('order-status')).toHaveText('Shipped')
  await noHorizontalScroll(page)
  await page.goto('/orders')
  await expect(page.getByRole('link', { name: order.number })).toBeVisible()
  await noHorizontalScroll(page)
  await page.goto('/supplier-orders')
  await expect(page.getByRole('tab', { name: /To confirm/ })).toBeVisible()
  await noHorizontalScroll(page)
  done()
})
