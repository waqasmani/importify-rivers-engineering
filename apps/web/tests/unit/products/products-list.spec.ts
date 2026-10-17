import { afterEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import ProductsView from '@/views/products/ProductsView.vue'
import ImportQueueView from '@/views/products/ImportQueueView.vue'
import ConfirmHost from '@/components/ui/ConfirmHost.vue'
import { apiError, mockApi, mountView, settle } from '../helpers'
import { PRODUCT, productDetail } from './fixtures'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

const row = productDetail()
const listRoutes = {
  'GET /products': { items: [row], total: 1, limit: 50, offset: 0 },
  'GET /products/facets': { domains: [{ source_domain: 'shop.example.com', product_count: 3 }], categories: [{ category: 'Outdoor > Drinkware', product_count: 2 }] },
  'GET /tags': { items: [{ id: 't1', name: 'flasks', color: '', created_at: '', usage_count: 1 }] },
  'GET /settings': { entity_labels: {}, imports: { default_currency: 'EUR' } },
  'GET /custom-fields': { items: [] },
}

function lastListQuery(calls: { method: string; path: string; query: URLSearchParams }[]) {
  return [...calls].reverse().find((c) => c.method === 'GET' && c.path === '/products')!.query
}

describe('ProductsView', () => {
  it('filters by facet chips and maps column sorting onto the server sort keys', async () => {
    const api = mockApi(listRoutes)
    const { wrapper, router } = await mountView(ProductsView, { path: '/products' })
    const chip = wrapper.get('[data-testid="facets"]').findAll('button').find((b) => b.text().startsWith('shop.example.com'))!
    expect(chip.text()).toContain('3')
    await chip.trigger('click')
    await settle()
    expect(router.currentRoute.value.query.source_domain).toBe('shop.example.com')
    expect(lastListQuery(api.calls).get('source_domain')).toBe('shop.example.com')
    await wrapper.get('[data-testid="facets"]').findAll('button').find((b) => b.text().startsWith('shop.example.com'))!.trigger('click')
    await settle()
    expect(router.currentRoute.value.query.source_domain).toBeUndefined()

    const header = (label: string) => wrapper.findAll('thead button').find((b) => b.text() === label)!
    await header('Margin').trigger('click')
    await settle()
    expect(router.currentRoute.value.query.sort).toBe('-margin')
    await header('Margin').trigger('click')
    await settle()
    expect(router.currentRoute.value.query.sort).toBeUndefined()
    await header('Price').trigger('click')
    await settle()
    expect(router.currentRoute.value.query.sort).toBe('price')
    await header('Price').trigger('click')
    await settle()
    expect(router.currentRoute.value.query.sort).toBe('-price')
    expect(lastListQuery(api.calls).get('sort')).toBe('-price')
  })

  it('applies a bulk review status and explains that nothing is published', async () => {
    const api = mockApi({ ...listRoutes, 'POST /products/bulk': { affected: 1 } })
    const { wrapper } = await mountView(ProductsView, { path: '/products' })
    await wrapper.get(`[data-testid="row-${PRODUCT}"] input[type="checkbox"]`).setValue(true)
    const bar = wrapper.get('[data-testid="bulk-actions"]')
    expect(bar.text()).toContain('1 selected')
    await bar.get('#bulk-action').setValue('set_review')
    expect(bar.text()).toContain('Nothing is published or sent anywhere')
    await bar.get('#bulk-review').setValue('approved')
    await bar.findAll('button').find((b) => b.text() === 'Apply')!.trigger('click')
    await settle()
    expect(api.calls.find((c) => c.method === 'POST')!.body).toEqual({ action: 'set_review', ids: [PRODUCT], value: 'approved' })
    expect(wrapper.find('[data-testid="bulk-actions"]').exists()).toBe(false)
  })

  it('confirms bulk deletion and only offers allowed actions', async () => {
    const api = mockApi({ ...listRoutes, 'POST /products/bulk': { affected: 1 } })
    const Host = defineComponent({ render: () => h('div', [h(ProductsView), h(ConfirmHost)]) })
    const { wrapper } = await mountView(Host, { path: '/products', permissions: ['products.read', 'products.write', 'products.delete'] })
    await wrapper.get(`[data-testid="row-${PRODUCT}"] input[type="checkbox"]`).setValue(true)
    const options = wrapper.get('#bulk-action').findAll('option').map((o) => o.attributes('value'))
    expect(options).toContain('delete')
    expect(options).not.toContain('set_review')
    expect(options).not.toContain('set_supplier')
    await wrapper.get('#bulk-action').setValue('delete')
    await wrapper.get('[data-testid="bulk-actions"]').findAll('button').find((b) => b.text() === 'Apply')!.trigger('click')
    await settle()
    expect(api.calls.some((c) => c.method === 'POST')).toBe(false)
    ;(document.querySelector('[data-testid="confirm-ok"]') as HTMLButtonElement).click()
    await settle()
    expect(api.calls.find((c) => c.method === 'POST')!.body).toEqual({ action: 'delete', ids: [PRODUCT], value: null })
  })

  it('hides export, CSV import, creation and selection without the permissions', async () => {
    mockApi(listRoutes)
    const { wrapper } = await mountView(ProductsView, { path: '/products', permissions: ['products.read'] })
    const buttons = wrapper.findAll('button').map((b) => b.text())
    expect(buttons).not.toContain('Export')
    expect(buttons).not.toContain('Import CSV')
    expect(buttons.some((b) => b.includes('New product'))).toBe(false)
    expect(wrapper.find(`[data-testid="row-${PRODUCT}"] input[type="checkbox"]`).exists()).toBe(false)
    expect(wrapper.text()).toContain('Trailblazer 2L Insulated Flask')
    expect(wrapper.text()).toContain('€39.95')
  })

  it('creates a product from the modal and shows field errors from the server', async () => {
    let attempts = 0
    const api = mockApi({
      ...listRoutes,
      'POST /products': () => (++attempts === 1 ? apiError(422, 'validation', 'Invalid input', { gtin: 'must be a valid GTIN-8, 12, 13 or 14 with a correct check digit' }) : productDetail({ id: 'new-product' })),
    })
    const { wrapper, router } = await mountView(ProductsView, { path: '/products' })
    await wrapper.findAll('button').find((b) => b.text().includes('New product'))!.trigger('click')
    await settle()
    const form = wrapper.get('[data-testid="product-form"]')
    await form.find('input').setValue('Desk Lamp')
    await form.find('input[inputmode="numeric"]').setValue('4006381333932')
    expect(form.text()).toContain('not a valid GTIN')
    await form.trigger('submit')
    await settle()
    expect(wrapper.get('[data-testid="product-form"]').text()).toContain('must be a valid GTIN-8, 12, 13 or 14')
    expect(api.calls.find((c) => c.method === 'POST')!.body).toMatchObject({ title: 'Desk Lamp', gtin: '4006381333932', status: 'draft', price: null, price_currency: null })
    await wrapper.get('[data-testid="product-form"]').find('input[inputmode="numeric"]').setValue('4006381333931')
    await wrapper.get('[data-testid="product-form"]').trigger('submit')
    await settle()
    expect(router.currentRoute.value.path).toBe('/products/new-product')
  })
})

describe('ImportQueueView', () => {
  it('highlights imports that need a decision and filters by status', async () => {
    const item = (id: string, status: string, n: number) => ({
      id, status, source: 'extension', decision: null, payload: {}, normalized: { title: `Bottle ${id}`, source_url: 'https://shop.example.com/p/b' }, warnings: [], error: '',
      duplicate_candidates: Array.from({ length: n }, (_, i) => ({ id: `d${i}`, label: 'x', score: 1, reasons: [] })), product_id: null, duplicate_of_id: null, captured_page_id: null,
      device_id: 'dev1', created_by: null, reviewed_by: null, created_at: '2026-10-01T10:00:00Z', processed_at: null, created_by_name: 'Pat',
    })
    const api = mockApi({
      'GET /imports': (c) => {
        const st = c.query.get('status')
        const all = [item('a', 'duplicate', 2), item('b', 'imported', 0)]
        const items = st ? all.filter((i) => i.status === st) : all
        return { items, total: items.length, limit: 50, offset: 0 }
      },
      'GET /extension/devices': { items: [{ id: 'dev1', name: 'Work laptop', browser: 'Chrome', extension_version: '1.0', user_id: 'u', user_name: 'Pat', user_email: 'p@x', created_at: '', last_seen_at: null, last_ip: '', revoked_at: null }] },
    })
    const { wrapper, router } = await mountView(ImportQueueView, { path: '/imports' })
    expect(wrapper.get('[data-testid="needs-decision-banner"]').text()).toContain('1 import needs a decision')
    const first = wrapper.get('[data-testid="row-a"]')
    expect(first.text()).toContain('Needs decision')
    expect(first.text()).toContain('Possible duplicate')
    expect(first.text()).toContain('Work laptop')
    expect(first.get('a').attributes('href')).toBe('/imports/a')
    expect(wrapper.get('[data-testid="row-b"]').text()).not.toContain('Needs decision')
    await wrapper.findAll('button').find((b) => b.text() === 'Show possible duplicates')!.trigger('click')
    await settle()
    expect(router.currentRoute.value.query.status).toBe('duplicate')
    expect(wrapper.findAll('tbody tr')).toHaveLength(1)
    expect(api.calls.some((c) => c.path === '/imports' && c.query.get('status') === 'duplicate' && c.query.get('limit') === '50')).toBe(true)
  })
})
