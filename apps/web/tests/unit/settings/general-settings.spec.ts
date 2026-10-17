import { afterEach, describe, expect, it, vi } from 'vitest'
import GeneralSettingsView from '@/views/settings/GeneralSettingsView.vue'
import { apiError, mockApi, mountView, settle } from '../helpers'

const settings = {
  organization: { name: 'Rivers' },
  entity_labels: { supplier: { singular: 'Supplier', plural: 'Suppliers' } },
  imports: { default_currency: 'USD', title_similarity_threshold: 0.55, auto_link_supplier_by_domain: true },
  capture: { store_html_snapshots: false, max_snapshot_kb: 1024, retention_days: 0 },
  ai: { enabled: true },
}

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

describe('GeneralSettingsView', () => {
  it('saves one section at a time and shows server errors next to the field', async () => {
    const api = mockApi({
      'GET /settings': settings,
      'PUT /settings/entity_labels': (c) => {
        const body = c.body as Record<string, { singular: string; plural: string }>
        if (body.supplier!.plural === '') return apiError(422, 'validation_failed', 'Invalid input.', { 'supplier.plural': 'must be between 1 and 40 characters' })
        return { key: 'entity_labels', value: body, updated_at: '', updated_by: null }
      },
      'PUT /settings/capture': (c) => ({ key: 'capture', value: c.body, updated_at: '', updated_by: null }),
    })
    const { wrapper } = await mountView(GeneralSettingsView)
    await wrapper.find('[data-testid="label-supplier-singular"]').setValue('Vendor')
    const plural = wrapper.find('[data-testid="label-supplier-plural"]')
    await plural.setValue('')
    plural.element.removeAttribute('required')
    await wrapper.find('[data-testid="label-supplier-plural"]').element.closest('form')!.dispatchEvent(new Event('submit'))
    await settle()
    const put = api.calls.find((c) => c.path === '/settings/entity_labels')!
    expect(Object.keys(put.body as object).sort()).toEqual(['company', 'contact', 'lead', 'opportunity', 'product', 'supplier'])
    expect((put.body as Record<string, object>).supplier).toEqual({ singular: 'Vendor', plural: '' })
    expect((put.body as Record<string, object>).company).toEqual({ singular: 'Company', plural: 'Companies' })
    const field = plural.element.closest('div')!.parentElement!
    expect(field.textContent).toContain('must be between 1 and 40 characters')
    expect(api.calls.filter((c) => c.method === 'PUT')).toHaveLength(1)

    await wrapper.findAll('button').find((b) => b.text() === 'Save capture settings')!.trigger('click')
    await settle()
    const cap = api.calls.find((c) => c.path === '/settings/capture')!
    expect(cap.body).toEqual({ store_html_snapshots: false, max_snapshot_kb: 1024, retention_days: 0 })
  })
})
