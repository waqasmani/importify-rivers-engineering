import { afterEach, describe, expect, it, vi } from 'vitest'
import NewImportView from '@/views/products/NewImportView.vue'
import ImportDetailView from '@/views/products/ImportDetailView.vue'
import type { Schemas } from '@/api/client'
import { mockApi, mountView, settle } from '../helpers'
import { evaluation } from '../products/fixtures'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

const IMPORT_ID = '33333333-3333-4333-8333-333333333333'

// 20 EUR at 1.10 = 22 USD, +50% = 33 → ending .99 = 32.99.
const pricing: Schemas['ImportPricing'] = {
  base_currency: 'USD',
  product: evaluation({
    currency: 'USD', cost: '20', cost_currency: 'EUR', rate: '1.1', base_cost: '22', price: '32.99', compare_at_price: '42.99', margin_amount: '10.99', margin_percent: '33.31',
    rule: { id: 'r1', name: 'Shop markup', is_default: false },
    steps: [
      { kind: 'conversion', label: '20 EUR converted at 1 EUR = 1.1 USD', amount: '22' },
      { kind: 'cost', label: 'Cost', amount: '22' },
      { kind: 'markup', label: '+ 50% markup', amount: '33' },
      { kind: 'rounding', label: 'Rounded to the nearest price ending in .99', amount: '32.99' },
      { kind: 'price', label: 'Selling price', amount: '32.99' },
    ],
  }),
  variants: [
    { index: 0, title: 'S', evaluation: evaluation({ currency: 'USD', base_cost: '22', price: '32.99' }) },
    { index: 1, title: 'L', evaluation: evaluation({ status: 'no_rate', message: 'No exchange rate from GBP to USD is recorded.', price: null, rule: null, base_cost: null }) },
  ],
}

const normalized = {
  title: 'Enamel Mug', normalized_title: 'enamel mug', source_url: 'https://shop.example.com/p/mug', canonical_url: 'https://shop.example.com/p/mug', source_domain: 'shop.example.com',
  description: '', brand: '', sku: 'M1', model: '', mpn: '', gtin: null, identifiers: {}, price: '20', price_currency: 'EUR', availability: 'in_stock', category: '', images: [],
  variants: [], specifications: {}, supplier_name: '', supplier_domain: '', extraction_method: 'manual', adapter_key: '', confidence: null, evidence: {}, warnings: [],
}
const base = { 'GET /settings': { entity_labels: {}, imports: { default_currency: 'USD' } }, 'GET /extension/devices': { items: [] } }

describe('import review pricing', () => {
  it('shows the selling price the rules give a capture before it is stored', async () => {
    mockApi({
      ...base,
      'POST /imports/products/check': { normalized, warnings: [], errors: {}, duplicates: [], definite_duplicate: false, suggested_supplier: null, pricing },
    })
    const { wrapper } = await mountView(NewImportView, { path: '/imports/new' })
    const form = wrapper.get('[data-testid="payload-form"]')
    const inputs = form.findAll('input')
    await inputs[0]!.setValue('Enamel Mug')
    await inputs[1]!.setValue('https://shop.example.com/p/mug')
    await wrapper.findAll('form')[0]!.trigger('submit')
    await settle()
    const card = wrapper.get('[data-testid="import-pricing"]')
    expect(card.get('[data-testid="evaluation-price"]').text()).toContain('32.99')
    expect(card.get('[data-testid="evaluation-compare-at"]').text()).toContain('42.99')
    expect(card.get('[data-testid="evaluation-rule"]').text()).toBe('Shop markup')
    expect(card.text()).toContain('20 EUR converted at 1 EUR = 1.1 USD')
    expect(card.text()).toContain('33.31%')
    expect(card.get('[data-testid="import-variant-price-0"]').text()).toContain('32.99')
    expect(card.get('[data-testid="import-variant-price-1"]').text()).toContain('No exchange rate')
  })

  it('shows the current rules’ price on an import record', async () => {
    mockApi({
      ...base,
      [`GET /imports/${IMPORT_ID}`]: {
        id: IMPORT_ID, source: 'extension', status: 'imported', decision: 'create_new', payload: { title: 'Enamel Mug' }, normalized, warnings: [], error: '', duplicate_candidates: [],
        product_id: '22222222-2222-4222-8222-222222222222', duplicate_of_id: null, captured_page_id: null, device_id: null, created_by: null, reviewed_by: null,
        created_at: '2026-10-01T10:00:00Z', processed_at: '2026-10-01T10:00:00Z', pricing,
      },
    })
    const { wrapper } = await mountView(ImportDetailView, { path: `/imports/${IMPORT_ID}`, pattern: '/imports/:id' })
    const card = wrapper.get('[data-testid="import-pricing"]')
    expect(card.text()).toContain('Selling price with the current rules')
    expect(card.text()).toContain('open the product for its actual price')
    expect(card.get('[data-testid="evaluation-price"]').text()).toContain('32.99')
  })
})
