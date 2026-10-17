import { afterEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import CapturesView from '@/views/products/CapturesView.vue'
import SupplierLinks from '@/components/products/SupplierLinks.vue'
import ConfirmHost from '@/components/ui/ConfirmHost.vue'
import { mockApi, mountView, settle } from '../helpers'
import { CAPTURE, PRODUCT, SUPPLIER } from './fixtures'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

const captureRow = {
  id: CAPTURE, url: 'https://shop.example.com/p/flask', canonical_url: null, domain: 'shop.example.com', title: 'Flask page', description: '', page_type: 'product',
  adapter_key: 'json-ld', extraction_method: 'json-ld', warnings: [], has_snapshot: true, selection_count: 2, captured_by: null, device_id: null,
  captured_at: '2026-10-01T10:00:00Z', total: 1,
}

describe('CapturesView', () => {
  it('purges older captures after choosing a date and confirming', async () => {
    const api = mockApi({
      'GET /captures': { items: [captureRow], total: 1, limit: 50, offset: 0 },
      'GET /settings': { entity_labels: {}, capture: { retention_days: 30, store_html_snapshots: true, max_snapshot_kb: 512 } },
      'GET /extension/devices': { items: [] },
      'POST /captures/purge': { deleted: 4 },
    })
    const Host = defineComponent({ render: () => h('div', [h(CapturesView), h(ConfirmHost)]) })
    const { wrapper } = await mountView(Host, { path: '/captures' })
    expect(wrapper.text()).toContain('Flask page')
    expect(wrapper.text()).toContain('2 selections · snapshot')
    await wrapper.findAll('button').find((b) => b.text().includes('Purge old captures'))!.trigger('click')
    await settle()
    const dialog = document.querySelector('dialog[open]')!
    expect(dialog.textContent).toContain('Automatic retention is set to 30 days')
    const date = dialog.querySelector('input[type="date"]') as HTMLInputElement
    date.value = '2026-01-15'
    date.dispatchEvent(new Event('input'))
    dialog.querySelector('form')!.dispatchEvent(new Event('submit'))
    await settle()
    expect(api.calls.some((c) => c.method === 'POST')).toBe(false)
    expect(document.body.textContent).toContain('This cannot be undone.')
    ;(document.querySelector('[data-testid="confirm-ok"]') as HTMLButtonElement).click()
    await settle()
    const post = api.calls.find((c) => c.method === 'POST')!
    expect(post.path).toBe('/captures/purge')
    expect(new Date((post.body as { before: string }).before).getTime()).toBe(new Date('2026-01-15T00:00:00').getTime())
  })

  it('hides purge and delete without products.delete', async () => {
    mockApi({ 'GET /captures': { items: [captureRow], total: 1, limit: 50, offset: 0 }, 'GET /extension/devices': { items: [] } })
    const { wrapper } = await mountView(CapturesView, { path: '/captures', permissions: ['products.read'] })
    expect(wrapper.findAll('button').some((b) => b.text().includes('Purge'))).toBe(false)
    expect(wrapper.find(`button[aria-label^="Delete capture"]`).exists()).toBe(false)
    expect(wrapper.get(`[data-testid="row-${CAPTURE}"] a`).attributes('href')).toBe(`/captures/${CAPTURE}`)
  })
})

describe('SupplierLinks', () => {
  const link = {
    supplier_id: SUPPLIER, product_id: PRODUCT, supplier_name: 'Northwind Outdoor', supplier_country: 'DE', supplier_rating: null, supplier_sku: 'NW-1', cost: '12',
    currency: 'EUR', moq: 100, lead_time_days: 30, is_preferred: true, notes: '', created_at: '', updated_at: '',
  }

  it('edits and removes a supplier link through the supplier-product endpoints', async () => {
    const api = mockApi({
      [`GET /products/${PRODUCT}/suppliers`]: { items: [link] },
      'GET /settings': { entity_labels: {} },
      [`PUT /suppliers/${SUPPLIER}/products/${PRODUCT}`]: (c) => ({ ...link, ...(c.body as object) }),
      [`DELETE /suppliers/${SUPPLIER}/products/${PRODUCT}`]: () => undefined,
      [`GET /suppliers/${SUPPLIER}`]: { id: SUPPLIER, name: 'Northwind Outdoor' },
    })
    const Host = defineComponent({ render: () => h('div', [h(SupplierLinks, { productId: PRODUCT, defaultCurrency: 'EUR', canEdit: true }), h(ConfirmHost)]) })
    const { wrapper } = await mountView(Host)
    const text = wrapper.text().replace(/\s+/g, ' ')
    expect(text).toContain('SKU NW-1 · Cost €12.00 · MOQ 100 · 30 days lead time')
    await wrapper.get('button[aria-label="Edit link to Northwind Outdoor"]').trigger('click')
    await settle()
    const dialog = document.querySelector('dialog[open]')!
    const lead = [...dialog.querySelectorAll('label')].find((l) => l.textContent?.includes('Lead time'))!
    const input = dialog.querySelector(`[id="${lead.htmlFor}"]`) as HTMLInputElement
    input.value = '21'
    input.dispatchEvent(new Event('input'))
    dialog.querySelector('form')!.dispatchEvent(new Event('submit'))
    await settle()
    expect(api.calls.find((c) => c.method === 'PUT')!.body).toEqual({ supplier_sku: 'NW-1', cost: '12', currency: 'EUR', moq: 100, lead_time_days: 21, is_preferred: true, notes: '' })

    await wrapper.get('button[aria-label="Remove Northwind Outdoor"]').trigger('click')
    await settle()
    ;(document.querySelector('[data-testid="confirm-ok"]') as HTMLButtonElement).click()
    await settle()
    expect(api.calls.some((c) => c.method === 'DELETE' && c.path === `/suppliers/${SUPPLIER}/products/${PRODUCT}`)).toBe(true)
  })
})
