import { afterEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import ProductDetailView from '@/views/products/ProductDetailView.vue'
import ProductsView from '@/views/products/ProductsView.vue'
import ConfirmHost from '@/components/ui/ConfirmHost.vue'
import type { Schemas } from '@/api/client'
import { mockApi, mountView, settle, type Call } from '../helpers'
import { PRODUCT, evaluation, productDetail, productPageRoutes, variant } from '../products/fixtures'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

const DetailHost = defineComponent({ props: ['id'], setup: (p) => () => h('div', [h(ProductDetailView, { id: p.id as string }), h(ConfirmHost)]) })
const ListHost = defineComponent({ render: () => h('div', [h(ProductsView), h(ConfirmHost)]) })

function repriceResult(dryRun: boolean): Schemas['RepriceResult'] {
  const snap = (price: string | null, cost = '14'): Schemas['PriceSnapshot'] => ({
    cost, cost_currency: 'EUR', price, price_currency: 'EUR', compare_at_price: null, pricing_rule_id: null, pricing_rule_name: 'Outdoor markup',
  })
  return {
    dry_run: dryRun, base_currency: 'EUR',
    summary: { products: 2, changed: 1, unchanged: 1, locked: 1, not_priced: 0, variants: 1, variants_changed: 1 },
    items: [
      {
        product_id: PRODUCT, title: 'Trailblazer 2L Insulated Flask', status: 'repriced', changed: true, message: '', warnings: [], before: snap('39.95'), after: snap('44.95'),
        variants: [{ variant_id: variant().id, title: 'Forest', status: 'repriced', changed: true, message: '', before: snap('39.95'), after: snap('44.95') }],
      },
      { product_id: 'locked-1', title: 'Locked lamp', status: 'locked', changed: false, message: 'The price is locked. The rules would give 30.00 EUR.', warnings: [], before: snap('25'), after: snap('25'), variants: [] },
    ],
  }
}

describe('product price breakdown', () => {
  it('shows source price → cost → rule → selling price with margin and the calculation steps', async () => {
    mockApi(productPageRoutes(productDetail({ compare_at_price: '49.95' })))
    const { wrapper } = await mountView(DetailHost, { path: `/products/${PRODUCT}`, pattern: '/products/:id' })
    const b = wrapper.get('[data-testid="price-breakdown"]')
    expect(b.get('[data-testid="source-price"]').text()).toContain('14.00')
    expect(b.get('[data-testid="cost"]').text()).toContain('14.00')
    expect(b.text()).toContain('from the source price')
    expect(b.text()).toContain('Rule: Outdoor markup')
    expect(b.get('[data-testid="selling-price"]').text()).toContain('39.95')
    expect(b.text()).toContain('compare-at')
    expect(b.text()).toContain('49.95')
    expect(b.text()).toContain('64.96%')
    expect(b.text()).toContain('Rounded to the nearest price ending in .95')
    expect(b.find('[data-testid="pending-change"]').exists()).toBe(false)
    // Variants show source price, cost and price.
    expect(wrapper.get(`[data-testid="variant-${variant().id}"]`).text()).toContain('39.95')
  })

  it('locks a manual price and unlocks it after a confirmation that names the rule price', async () => {
    const locked = productDetail({ price: '45', price_locked: true, pricing_rule_id: null, pricing_rule_name: undefined,
      pricing: { ...productDetail().pricing, product: { status: 'locked', changed: false, evaluation: evaluation({ price: '39.95' }) } } })
    let current = productDetail()
    const api = mockApi({
      ...productPageRoutes(current),
      [`GET /products/${PRODUCT}`]: () => current,
      [`PUT /products/${PRODUCT}/pricing`]: (c: Call) => {
        current = (c.body as { price_locked: boolean }).price_locked ? locked : productDetail()
        return current
      },
    })
    const { wrapper } = await mountView(DetailHost, { path: `/products/${PRODUCT}`, pattern: '/products/:id' })
    await wrapper.findAll('button').find((b) => b.text().includes('Set a manual price'))!.trigger('click')
    const lockForm = wrapper.get('[data-testid="lock-form"]')
    expect((lockForm.get('[data-testid="lock-price"]').element as HTMLInputElement).value).toBe('39.95')
    await lockForm.get('[data-testid="lock-price"]').setValue('45')
    await lockForm.trigger('submit')
    await settle()
    expect(api.calls.find((c) => c.method === 'PUT')!.body).toEqual({ price_locked: true, price: '45', price_currency: 'EUR', compare_at_price: null })
    const b = wrapper.get('[data-testid="price-breakdown"]')
    expect(b.text()).toContain('Manual price')
    expect(b.get('[data-testid="locked-note"]').text()).toContain('39.95')

    await wrapper.findAll('button').find((b) => b.text().includes('Unlock price'))!.trigger('click')
    await settle()
    expect(document.body.textContent).toContain('is replaced by €39.95 from the rule “Outdoor markup”')
    ;(document.querySelector('[data-testid="confirm-ok"]') as HTMLButtonElement).click()
    await settle()
    const puts = api.calls.filter((c) => c.method === 'PUT')
    expect(puts[1]!.body).toEqual({ price_locked: false })
  })

  it('flags a pending change and recalculates on demand', async () => {
    const stale = productDetail({ pricing: { ...productDetail().pricing, product: { status: 'repriced', changed: true, evaluation: evaluation({ price: '44.95' }) } } })
    const api = mockApi({ ...productPageRoutes(stale), 'POST /products/reprice': repriceResult(false) })
    const { wrapper } = await mountView(DetailHost, { path: `/products/${PRODUCT}`, pattern: '/products/:id' })
    expect(wrapper.get('[data-testid="pending-change"]').text()).toContain('44.95')
    await wrapper.findAll('button').find((b) => b.text().includes('Recalculate'))!.trigger('click')
    await settle()
    expect(api.calls.find((c) => c.path === '/products/reprice')!.body).toEqual({ ids: [PRODUCT], dry_run: false })
    expect(api.calls.filter((c) => c.method === 'GET' && c.path === `/products/${PRODUCT}`).length).toBeGreaterThan(1)
  })

  it('hides pricing actions from users who cannot edit products', async () => {
    mockApi(productPageRoutes(productDetail()))
    const { wrapper } = await mountView(DetailHost, { path: `/products/${PRODUCT}`, pattern: '/products/:id', permissions: ['products.read'] })
    expect(wrapper.get('[data-testid="price-breakdown"]').text()).not.toContain('Recalculate')
  })
})

describe('bulk reprice', () => {
  const listRoutes = (extra: Record<string, unknown>) => ({
    'GET /products': { items: [productDetail()], total: 1, limit: 50, offset: 0 },
    'GET /products/facets': { domains: [], categories: [] },
    'GET /tags': { items: [] },
    'GET /settings': { entity_labels: {}, imports: { default_currency: 'EUR' } },
    'GET /custom-fields': { items: [] },
    ...extra,
  })

  it('previews a dry run for the selected products, then applies it', async () => {
    const api = mockApi(listRoutes({ 'POST /products/reprice': (c: Call) => repriceResult((c.body as { dry_run: boolean }).dry_run) }))
    const { wrapper } = await mountView(ListHost, { path: '/products' })
    await wrapper.get(`[data-testid="row-${PRODUCT}"] input[type="checkbox"]`).setValue(true)
    const bar = wrapper.get('[data-testid="bulk-actions"]')
    await bar.get('#bulk-action').setValue('reprice')
    expect(bar.text()).toContain('Locked manual prices are kept')
    await bar.findAll('button').find((b) => b.text() === 'Preview new prices')!.trigger('click')
    await settle()
    expect(api.calls.find((c) => c.path === '/products/reprice')!.body).toEqual({ ids: [PRODUCT], dry_run: true })
    const dialog = wrapper.get('[data-testid="reprice-dialog"]')
    expect(dialog.get('[data-testid="summary-changed"]').text()).toBe('1')
    const row = dialog.get(`[data-testid="reprice-${PRODUCT}"]`)
    expect(row.text()).toContain('39.95')
    expect(row.text()).toContain('44.95')
    expect(dialog.get(`[data-testid="reprice-variant-${variant().id}"]`).text()).toContain('44.95')
    // The locked product is hidden while "only changes" is on.
    expect(dialog.text()).not.toContain('Locked lamp')
    await dialog.find('input[type="checkbox"]').setValue(false)
    expect(dialog.text()).toContain('Locked lamp')
    expect(dialog.text()).toContain('The rules would give 30.00 EUR')

    await wrapper.get('[data-testid="reprice-apply"]').trigger('click')
    await settle()
    const posts = api.calls.filter((c) => c.path === '/products/reprice')
    expect(posts[1]!.body).toEqual({ ids: [PRODUCT], dry_run: false })
    expect(wrapper.find('[data-testid="bulk-actions"]').exists()).toBe(false)
  })

  it('reprices everything matching the current filters', async () => {
    const api = mockApi(listRoutes({ 'POST /products/reprice': (c: Call) => repriceResult((c.body as { dry_run: boolean }).dry_run) }))
    const { wrapper } = await mountView(ListHost, { path: '/products?source_domain=shop.example.com&q=flask' })
    await wrapper.findAll('button').find((b) => b.text().includes('Reprice matching'))!.trigger('click')
    await settle()
    expect(api.calls.find((c) => c.path === '/products/reprice')!.body).toEqual({
      filter: { q: 'flask', status: '', review_status: '', source_domain: 'shop.example.com', category: '', availability: '', supplier_id: null, tag_id: null },
      dry_run: true,
    })
  })
})
