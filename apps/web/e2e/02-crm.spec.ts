import { expect, test } from '@playwright/test'
import { apiGet, signIn, watchForErrors } from './helpers'

test.beforeEach(async ({ page }) => signIn(page))

test('create a company, add a note and a tag, then find it in search', async ({ page }, testInfo) => {
  const done = watchForErrors(page, testInfo)
  await page.goto('/companies')
  await page.getByRole('button', { name: 'New company' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByLabel('Name').fill('Ningbo Bright Housewares')
  await dialog.getByLabel('Website').fill('https://bright-housewares.example')
  await dialog.getByRole('button', { name: 'Create company' }).click()
  await expect(page.getByRole('heading', { name: 'Ningbo Bright Housewares' })).toBeVisible()

  // Note on the record.
  await page.getByLabel('New note').fill('Met at the Canton Fair; sample request pending.')
  await page.getByRole('button', { name: 'Add note' }).click()
  await expect(page.getByTestId('note').first()).toContainText('Canton Fair')

  // Tag via the tag editor.
  await page.getByRole('button', { name: 'Edit tags' }).click()
  await page.getByLabel('Add tag').fill('kitchenware')
  await page.getByLabel('Add tag').press('Enter')
  await page.getByRole('button', { name: 'Save tags' }).click()
  await expect(page.getByText('kitchenware').first()).toBeVisible()

  const companies = await apiGet<{ items: { id: string; name: string; tags: { name: string }[] }[] }>(page, '/companies?q=Bright')
  expect(companies.items).toHaveLength(1)
  expect(companies.items[0]!.tags.map((t) => t.name)).toContain('kitchenware')
  const notes = await apiGet<{ items: { body: string }[] }>(page, `/notes?entity_type=company&entity_id=${companies.items[0]!.id}`)
  expect(notes.items[0]!.body).toContain('Canton Fair')

  // Global search finds it.
  await page.getByRole('combobox', { name: 'Search everything' }).fill('Bright House')
  await expect(page.getByRole('option', { name: /Ningbo Bright Housewares/ })).toBeVisible()
  done()
})

test('tasks: create a task with a due date', async ({ page }, testInfo) => {
  const done = watchForErrors(page, testInfo)
  await page.goto('/tasks')
  await page.getByRole('button', { name: /New task/ }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByLabel('Title').fill('Ask for MOQ and lead time')
  await dialog.getByLabel('Due').fill('2030-01-15T10:00')
  await dialog.getByRole('button', { name: 'Create task' }).click()
  await expect(page.getByText('Ask for MOQ and lead time').first()).toBeVisible()
  const tasks = await apiGet<{ items: { id: string; title: string; status: string }[] }>(page, '/tasks?q=MOQ')
  expect(tasks.items[0]?.status).toBe('open')
  done()
})
