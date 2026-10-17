import { expect, test } from '@playwright/test'
import { apiGet, signIn, watchForErrors } from './helpers'

test.beforeEach(async ({ page }) => signIn(page))

test('manual product import goes to review, and a repeat import is caught as a duplicate', async ({ page }, testInfo) => {
  const done = watchForErrors(page, testInfo)
  await page.goto('/imports/new')
  await page.getByLabel('Product page URL').fill('https://shop.example.com/products/steel-bottle-750')
  await page.getByLabel('Title').fill('Steel bottle 750 ml')
  await page.getByLabel('Price as shown').fill('$24.99')
  await page.getByLabel('SKU').fill('SB-750')
  await page.getByRole('button', { name: 'Check data' }).click()
  await expect(page.getByText('24.99').first()).toBeVisible()
  await page.getByRole('button', { name: /Import|Create/ }).last().click()
  await expect(page.getByRole('button', { name: 'Import another product' })).toBeVisible()

  const products = await apiGet<{ items: { id: string; title: string; review_status: string; status: string }[] }>(page, '/products?q=Steel')
  expect(products.items).toHaveLength(1)
  expect(products.items[0]).toMatchObject({ review_status: 'pending', status: 'draft' })

  // The same page again is parked for a decision instead of creating a second product.
  await page.getByRole('button', { name: 'Import another product' }).click()
  await page.getByLabel('Product page URL').fill('https://shop.example.com/products/steel-bottle-750')
  await page.getByLabel('Title').fill('Steel bottle 750 ml')
  await page.getByLabel('SKU').fill('SB-750')
  await page.getByRole('button', { name: 'Check data' }).click()
  await expect(page.getByText(/duplicate/i).first()).toBeVisible()
  done()
})

test('approving a product only changes its review status inside REA', async ({ page }, testInfo) => {
  const done = watchForErrors(page, testInfo)
  const products = await apiGet<{ items: { id: string }[] }>(page, '/products?q=Steel')
  await page.goto(`/products/${products.items[0]!.id}`)
  await expect(page.getByText(/never publishes products/i).first()).toBeVisible()
  await page.getByRole('button', { name: /^Approve/ }).first().click()
  const confirm = page.getByTestId('confirm-ok')
  if (await confirm.isVisible().catch(() => false)) await confirm.click()
  await expect.poll(async () => (await apiGet<{ review_status: string }>(page, `/products/${products.items[0]!.id}`)).review_status).toBe('approved')
  done()
})
