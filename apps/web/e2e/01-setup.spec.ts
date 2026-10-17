import { expect, test } from '@playwright/test'
import { OWNER, SETUP_TOKEN, watchForErrors } from './helpers'

test('first-run setup creates the organization and signs the owner in', async ({ page }, testInfo) => {
  const done = watchForErrors(page, testInfo)
  await page.goto('/')
  await expect(page).toHaveURL(/\/setup$/)
  await page.getByLabel('Setup token').fill(SETUP_TOKEN)
  await page.getByLabel('Organization name').fill('E2E Trading')
  await page.getByLabel('First workspace').fill('Sourcing')
  await page.getByLabel('Your name').fill(OWNER.name)
  await page.getByLabel('Email').fill(OWNER.email)
  await page.getByLabel(/^Password/).fill(OWNER.password)
  await page.getByLabel('Confirm password').fill(OWNER.password)
  await page.getByRole('button', { name: 'Create organization' }).click()
  await expect(page.getByRole('heading', { name: `Welcome, ${OWNER.name}` })).toBeVisible()

  // Setup cannot run twice.
  await page.goto('/setup')
  await expect(page).toHaveURL(/\/$/)
  done()
})

test('sign out and back in; protected pages redirect to sign-in', async ({ page }, testInfo) => {
  const done = watchForErrors(page, testInfo)
  await page.goto('/companies')
  await expect(page).toHaveURL(/\/login\?next=(%2F|\/)companies/)
  await page.getByLabel('Email').fill(OWNER.email)
  await page.getByLabel('Password').fill('wrong password entirely')
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page.getByRole('alert')).toBeVisible()
  await page.getByLabel('Password').fill(OWNER.password)
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page).toHaveURL(/\/companies$/)
  await page.getByRole('button', { name: OWNER.name }).click()
  await page.getByRole('menuitem', { name: 'Sign out' }).click()
  await expect(page).toHaveURL(/\/login/)
  done()
})

test('the dashboard is served with security headers', async ({ request }) => {
  const res = await request.get('/')
  const h = res.headers()
  expect(h['content-security-policy']).toContain("script-src 'self'")
  expect(h['x-frame-options']).toBe('DENY')
  expect(h['x-content-type-options']).toBe('nosniff')
  const api = await request.get('/api/v1/auth/me')
  expect(api.status()).toBe(401)
})
