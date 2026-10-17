import { afterEach, describe, expect, it, vi } from 'vitest'
import CompaniesView from '@/views/crm/CompaniesView.vue'
import { mockApi, mountView, settle } from '../helpers'
import { baseRoutes, company, confirmOk, page, withConfirm } from './fixtures'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

const rows = [company('c1', 'Acme GmbH', { tags: [{ id: 'tag-1', name: 'vip', color: '#ff0000' }] }), company('c2', 'Beta Ltd')]

describe('CompaniesView', () => {
  it('sends URL filters and sort keys from the handler allow-list to the API', async () => {
    const api = mockApi({ ...baseRoutes, 'GET /companies': () => page(rows) })
    const { wrapper, router } = await mountView(CompaniesView, { path: '/companies?status=inactive&country=DE', pattern: '/companies' })
    const first = api.calls.find((c) => c.path === '/companies')!
    expect(first.query.get('status')).toBe('inactive')
    expect(first.query.get('country')).toBe('DE')
    expect(wrapper.text()).toContain('Acme GmbH')

    const type = wrapper.findAll('select').find((s) => s.find('option[value="customer"]').exists())!
    await type.setValue('customer')
    await settle()
    expect(router.currentRoute.value.query).toMatchObject({ status: 'inactive', country: 'DE', company_type: 'customer' })
    const last = api.calls.filter((c) => c.path === '/companies').at(-1)!
    expect(last.query.get('company_type')).toBe('customer')

    // Name toggles name → -name; Created uses created/-created.
    const sortBtn = () => wrapper.findAll('th button').find((b) => b.text().startsWith('Name'))!
    await sortBtn().trigger('click')
    await settle()
    expect(router.currentRoute.value.query.sort).toBe('name')
    await sortBtn().trigger('click')
    await settle()
    expect(router.currentRoute.value.query.sort).toBe('-name')
    expect(api.calls.filter((c) => c.path === '/companies').at(-1)!.query.get('sort')).toBe('-name')
  })

  it('applies a bulk status change to the selected rows', async () => {
    const api = mockApi({ ...baseRoutes, 'GET /companies': () => page(rows), 'POST /companies/bulk': { affected: 2 } })
    const { wrapper } = await mountView(withConfirm(CompaniesView), { path: '/companies', pattern: '/companies' })
    await wrapper.find('input[aria-label="Select all rows"]').setValue(true)
    expect(wrapper.find('[data-testid="bulk-bar"]').text()).toContain('2 selected')
    await wrapper.find('#bulk-action').setValue('set_status')
    await wrapper.find('#bulk-value').setValue('archived')
    await wrapper.findAll('[data-testid="bulk-bar"] button').find((b) => b.text() === 'Apply')!.trigger('click')
    await settle()
    const post = api.calls.find((c) => c.method === 'POST' && c.path === '/companies/bulk')
    expect(post?.body).toEqual({ action: 'set_status', ids: ['c1', 'c2'], value: 'archived' })
  })

  it('confirms bulk delete before calling the API', async () => {
    const api = mockApi({ ...baseRoutes, 'GET /companies': () => page(rows), 'POST /companies/bulk': { affected: 1 } })
    const { wrapper } = await mountView(withConfirm(CompaniesView), { path: '/companies', pattern: '/companies' })
    await wrapper.find('[data-testid="row-c2"] input[type="checkbox"]').setValue(true)
    await wrapper.find('#bulk-action').setValue('delete')
    await wrapper.findAll('[data-testid="bulk-bar"] button').find((b) => b.text() === 'Apply')!.trigger('click')
    await settle()
    expect(api.calls.some((c) => c.path === '/companies/bulk')).toBe(false)
    expect(document.body.textContent).toContain('Delete 1 company?')
    confirmOk()
    await settle()
    expect(api.calls.find((c) => c.path === '/companies/bulk')?.body).toEqual({ action: 'delete', ids: ['c2'], value: null })
  })

  it('saves the current filters and applies a saved filter to the URL', async () => {
    const saved = { id: 'sf1', name: 'German prospects', entity_type: 'company', filters: { country: 'DE', company_type: 'prospect' }, is_shared: true, user_id: 'someone-else', created_at: '', updated_at: '' }
    const api = mockApi({ ...baseRoutes, 'GET /saved-filters': { items: [saved] }, 'GET /companies': () => page(rows), 'POST /saved-filters': (c) => ({ ...(c.body as object), id: 'sf2' }) })
    const { wrapper, router } = await mountView(CompaniesView, { path: '/companies?status=active&offset=50', pattern: '/companies' })
    await wrapper.find('[data-testid="saved-filters"]').trigger('click')
    await settle()
    // Shared filters from other users cannot be deleted here.
    expect(wrapper.find('button[aria-label="Delete saved filter German prospects"]').exists()).toBe(false)
    await wrapper.findAll('button').find((b) => b.text().includes('Save current filters'))!.trigger('click')
    await settle()
    const form = document.querySelector('#saved-filter-form') as HTMLFormElement
    const name = form.querySelector('input') as HTMLInputElement
    name.value = 'Active'
    name.dispatchEvent(new Event('input'))
    ;(form.querySelector('input[type="checkbox"]') as HTMLInputElement).click()
    form.dispatchEvent(new Event('submit'))
    await settle()
    expect(api.calls.find((c) => c.method === 'POST' && c.path === '/saved-filters')?.body).toEqual({ entity_type: 'company', name: 'Active', filters: { status: 'active' }, is_shared: true })

    await wrapper.find('[data-testid="saved-filters"]').trigger('click')
    await settle()
    await wrapper.findAll('button').find((b) => b.text().includes('German prospects'))!.trigger('click')
    await settle()
    expect(router.currentRoute.value.query).toEqual({ country: 'DE', company_type: 'prospect' })
  })

  it('hides write, import and export actions without permission', async () => {
    mockApi({ ...baseRoutes, 'GET /companies': () => page(rows) })
    const { wrapper } = await mountView(CompaniesView, { path: '/companies', pattern: '/companies', permissions: ['crm.read'] })
    const text = wrapper.text()
    expect(text).not.toContain('New company')
    expect(text).not.toContain('Import CSV')
    expect(text).not.toContain('Export')
    expect(wrapper.find('input[aria-label="Select all rows"]').exists()).toBe(false)
  })
})
