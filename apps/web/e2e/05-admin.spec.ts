import { expect, test } from '@playwright/test'
import { OWNER, apiGet, signIn, watchForErrors } from './helpers'

test('a viewer sees data but no write actions, and admin pages are forbidden', async ({ page, browser }, testInfo) => {
  const done = watchForErrors(page, testInfo)
  await signIn(page)
  const roles = await apiGet<{ items: { id: string; key: string }[] }>(page, '/roles')
  const viewer = roles.items.find((r) => r.key === 'viewer')!
  const csrf = (await (await page.request.get('/api/v1/auth/me')).json()).csrf_token as string
  const created = await page.request.post('/api/v1/users', {
    headers: { 'X-CSRF-Token': csrf },
    data: { email: 'viewer@example.com', display_name: 'Vic Viewer', password: OWNER.password, role_id: viewer.id },
  })
  expect(created.status(), await created.text()).toBe(201)

  const ctx = await browser.newContext()
  const vp = await ctx.newPage()
  await signIn(vp, 'viewer@example.com', OWNER.password)
  await vp.goto('/companies')
  await expect(vp.getByText('Ningbo Bright Housewares').first()).toBeVisible()
  await expect(vp.getByRole('button', { name: 'New company' })).toHaveCount(0)
  await vp.goto('/settings/users')
  await expect(vp.getByRole('heading', { name: /do not have access/ })).toBeVisible()
  await expect(vp.getByRole('complementary', { name: 'Main navigation' }).getByRole('link', { name: 'Users' })).toHaveCount(0)
  await ctx.close()
  done()
})

test('settings: renaming a CRM label updates the navigation', async ({ page }, testInfo) => {
  const done = watchForErrors(page, testInfo)
  await signIn(page)
  const csrf = (await (await page.request.get('/api/v1/auth/me')).json()).csrf_token as string
  const settings = await apiGet<{ entity_labels: Record<string, { singular: string; plural: string }> }>(page, '/settings')
  const labels = { ...settings.entity_labels, supplier: { singular: 'Vendor', plural: 'Vendors' } }
  const res = await page.request.put('/api/v1/settings/entity_labels', { headers: { 'X-CSRF-Token': csrf }, data: labels })
  expect(res.status(), await res.text()).toBe(200)
  await page.goto('/')
  await expect(page.getByRole('complementary', { name: 'Main navigation' }).getByRole('link', { name: 'Vendors' })).toBeVisible()
  done()
})
