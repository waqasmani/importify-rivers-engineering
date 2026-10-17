import { afterEach, describe, expect, it, vi } from 'vitest'
import NewImportView from '@/views/products/NewImportView.vue'
import ImportDetailView from '@/views/products/ImportDetailView.vue'
import { useToast } from '@/composables/useToast'
import { apiError, mockApi, mountView, settle } from '../helpers'

const EXISTING = '22222222-2222-4222-8222-222222222222'
const IMPORT_ID = '33333333-3333-4333-8333-333333333333'
const SUPPLIER = '44444444-4444-4444-8444-444444444444'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

function normalized(title = 'Steel Bottle 750 ml') {
  return {
    title, normalized_title: title.toLowerCase(), source_url: 'https://shop.example.com/p/bottle', canonical_url: 'https://shop.example.com/p/bottle',
    source_domain: 'shop.example.com', description: '', brand: 'HydroCo', sku: 'HB-750', model: '', mpn: '', gtin: null, identifiers: {}, price: '24.99',
    price_currency: 'USD', availability: 'in_stock', category: '', images: [], variants: [], specifications: { Capacity: '750 ml' }, supplier_name: '',
    supplier_domain: '', extraction_method: 'manual', adapter_key: '', confidence: null, evidence: {}, warnings: ['no product images were found'],
  }
}

const duplicate = { id: EXISTING, label: 'Steel Bottle 750ml', score: 1, reasons: ['same product URL'], record: {} }

function checkResponse(over: object = {}) {
  return { normalized: normalized(), warnings: ['no product images were found'], errors: {}, duplicates: [duplicate], definite_duplicate: true, suggested_supplier: null, ...over }
}

function importRecord(status: string, over: object = {}) {
  return {
    id: IMPORT_ID, source: 'manual', status, decision: null, payload: { title: 'Steel Bottle 750 ml', url: 'https://shop.example.com/p/bottle' }, normalized: normalized(),
    warnings: [], error: '', duplicate_candidates: [{ id: EXISTING, label: 'Steel Bottle 750ml', score: 1, reasons: ['same product URL'] }], product_id: null,
    duplicate_of_id: EXISTING, captured_page_id: null, device_id: null, created_by: null, reviewed_by: null, created_at: '2026-10-01T10:00:00Z', processed_at: null, ...over,
  }
}

const base = { 'GET /settings': { entity_labels: {}, imports: { default_currency: 'USD' } }, 'GET /extension/devices': { items: [] } }

async function fillAndCheck(wrapper: Awaited<ReturnType<typeof mountView>>['wrapper']) {
  const form = wrapper.get('[data-testid="payload-form"]')
  const inputs = form.findAll('input')
  await inputs[0]!.setValue('Steel Bottle 750 ml')
  await inputs[1]!.setValue('https://shop.example.com/p/bottle')
  await form.find('input[placeholder="EUR"]').setValue('usd')
  await wrapper.findAll('button').find((b) => b.text() === 'Add specification')!.trigger('click')
  await form.find('input[aria-label="Specification 1"]').setValue('Capacity')
  await form.findAll('[data-testid="kv-row"]')[0]!.find('input[aria-label="Value 1"]').setValue('750 ml')
  await form.find('select').setValue('in_stock')
  await wrapper.findAll('form')[0]!.trigger('submit')
  await settle()
}

describe('NewImportView', () => {
  it('checks the payload, flags the definite duplicate and submits an update of the chosen product', async () => {
    const api = mockApi({
      ...base,
      'POST /imports/products/check': checkResponse(),
      'POST /imports/products': () =>
        new Response(JSON.stringify({ import: importRecord('updated', { product_id: EXISTING }), product: { id: EXISTING }, duplicates: [duplicate], status: 'updated', warnings: [] }), {
          status: 201, headers: { 'Content-Type': 'application/json' },
        }),
    })
    const { wrapper } = await mountView(NewImportView, { path: '/imports/new' })
    await fillAndCheck(wrapper)

    const check = api.calls.find((c) => c.path === '/imports/products/check')!
    expect(check.body).toMatchObject({
      source: 'manual',
      payload: { title: 'Steel Bottle 750 ml', url: 'https://shop.example.com/p/bottle', currency: 'USD', specifications: { Capacity: '750 ml' }, availability: 'https://schema.org/InStock' },
    })
    expect(wrapper.get('[data-testid="definite-duplicate"]').text()).toContain('Steel Bottle 750ml')
    expect(wrapper.get('[data-testid="check-warnings"]').text()).toContain('no product images were found')
    expect(wrapper.get('[data-testid="payload-comparison"]').text()).toContain('Normalized by REA')

    // The definite duplicate preselects "update existing" with the matched product.
    const decision = wrapper.get('[data-testid="decision-form"]')
    expect((decision.find('input[value="update_existing"]').element as HTMLInputElement).checked).toBe(true)
    expect((decision.find(`input[value="${EXISTING}"]`).element as HTMLInputElement).checked).toBe(true)
    await decision.find('input[placeholder="bottles, summer range"]').setValue('bottles, Bottles, summer')
    await decision.trigger('submit')
    await settle()

    const submit = api.calls.find((c) => c.method === 'POST' && c.path === '/imports/products')!
    expect(submit.body).toMatchObject({ decision: 'update_existing', target_product_id: EXISTING, source: 'manual', create_supplier: false, supplier_id: null, tag_names: ['bottles', 'summer'] })
    const outcome = wrapper.get('[data-testid="import-outcome"]')
    expect(outcome.text()).toContain('Existing product updated.')
    expect(outcome.get('[data-testid="outcome-product-link"]').attributes('href')).toBe(`/products/${EXISTING}`)
  })

  it('creates a new product with the suggested supplier and asks for a fresh check after edits', async () => {
    const api = mockApi({
      ...base,
      'POST /imports/products/check': checkResponse({ duplicates: [], definite_duplicate: false, suggested_supplier: { id: SUPPLIER, label: 'Shop Example', score: 1, reasons: ['same domain'], record: {} } }),
      'POST /imports/products': () =>
        new Response(JSON.stringify({ import: importRecord('imported', { product_id: EXISTING }), product: { id: EXISTING }, duplicates: [], status: 'imported', warnings: ['no product images were found'] }), {
          status: 201, headers: { 'Content-Type': 'application/json' },
        }),
    })
    const { wrapper } = await mountView(NewImportView, { path: '/imports/new' })
    await fillAndCheck(wrapper)
    expect(wrapper.get('[data-testid="suggested-supplier"]').text()).toContain('Shop Example')
    const decision = wrapper.get('[data-testid="decision-form"]')
    expect((decision.find('input[value="create_new"]').element as HTMLInputElement).checked).toBe(true)
    expect((decision.find('input[value="existing"]').element as HTMLInputElement).checked).toBe(true)

    // Editing the data makes the check stale and blocks submission until checked again.
    await wrapper.get('[data-testid="payload-form"]').findAll('input')[0]!.setValue('Steel Bottle 750 ml (blue)')
    expect(wrapper.text()).toContain('The product data changed after this check')
    const submitBtn = decision.findAll('button').find((b) => b.text() === 'Create draft product')!
    expect(submitBtn.attributes('disabled')).toBeDefined()
    await wrapper.findAll('form')[0]!.trigger('submit')
    await settle()
    expect(wrapper.text()).not.toContain('The product data changed after this check')

    await wrapper.get('[data-testid="decision-form"]').trigger('submit')
    await settle()
    const submit = api.calls.find((c) => c.method === 'POST' && c.path === '/imports/products')!
    expect(submit.body).toMatchObject({ decision: 'create_new', supplier_id: SUPPLIER, target_product_id: null })
    expect(wrapper.get('[data-testid="import-outcome"]').text()).toContain('Draft product created.')
    expect(wrapper.get('[data-testid="import-outcome"]').text()).toContain('no product images were found')
  })

  it('parks a duplicate in the queue (202) and resolves it in place', async () => {
    const api = mockApi({
      ...base,
      'POST /imports/products/check': checkResponse(),
      'POST /imports/products': () =>
        new Response(JSON.stringify({ import: importRecord('duplicate'), duplicates: [duplicate], status: 'needs_decision', warnings: [] }), { status: 202, headers: { 'Content-Type': 'application/json' } }),
      [`POST /imports/${IMPORT_ID}/resolve`]: () => importRecord('rejected', { decision: 'skip' }),
    })
    const { wrapper } = await mountView(NewImportView, { path: '/imports/new' })
    await fillAndCheck(wrapper)
    const decision = wrapper.get('[data-testid="decision-form"]')
    await decision.find('input[value="park"]').setValue(true)
    await decision.trigger('submit')
    await settle()
    expect(api.calls.find((c) => c.method === 'POST' && c.path === '/imports/products')!.body).toMatchObject({ decision: '', target_product_id: null })
    const outcome = wrapper.get('[data-testid="import-outcome"]')
    expect(outcome.text()).toContain('Needs decision')
    const resolve = outcome.get('[data-testid="resolve-form"]')
    await resolve.find('input[value="skip"]').setValue(true)
    await resolve.trigger('submit')
    await settle()
    expect(api.calls.find((c) => c.path === `/imports/${IMPORT_ID}/resolve`)!.body).toEqual({ decision: 'skip', target_product_id: null })
    expect(wrapper.get('[data-testid="import-outcome"]').text()).toContain('Import recorded as skipped')
  })

  it('shows normalization errors and server validation next to the fields', async () => {
    mockApi({
      ...base,
      'POST /imports/products/check': checkResponse({ errors: { title: 'is required; select the product title on the page if it was not detected' }, duplicates: [], definite_duplicate: false }),
    })
    const { wrapper } = await mountView(NewImportView, { path: '/imports/new' })
    await wrapper.findAll('form')[0]!.trigger('submit')
    await settle()
    expect(wrapper.get('[data-testid="check-errors"]').text()).toContain('title is required')
    const titleField = wrapper.get('[data-testid="payload-form"]').find('input')
    expect(titleField.classes()).toContain('input-invalid')
    expect(wrapper.get('[data-testid="payload-form"]').text()).toContain('is required; select the product title')
    expect(wrapper.find('[data-testid="decision-form"]').exists()).toBe(false)
  })
})

describe('ImportDetailView', () => {
  it('shows payload vs normalized and resolves a parked import by updating the matched product', async () => {
    const api = mockApi({
      ...base,
      [`GET /imports/${IMPORT_ID}`]: importRecord('duplicate'),
      [`POST /imports/${IMPORT_ID}/resolve`]: () => importRecord('updated', { decision: 'update_existing', product_id: EXISTING }),
    })
    const { wrapper } = await mountView(ImportDetailView, { path: `/imports/${IMPORT_ID}`, pattern: '/imports/:id' })
    expect(wrapper.text()).toContain('Decision needed')
    expect(wrapper.get('[data-testid="payload-comparison"]').text()).toContain('shop.example.com')
    const resolve = wrapper.get('[data-testid="resolve-form"]')
    expect((resolve.find(`input[value="${EXISTING}"]`).element as HTMLInputElement).checked).toBe(true)
    await resolve.trigger('submit')
    await settle()
    expect(api.calls.find((c) => c.method === 'POST')!.body).toEqual({ decision: 'update_existing', target_product_id: EXISTING })
    expect(wrapper.find('[data-testid="resolve-form"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="import-product-link"]').attributes('href')).toBe(`/products/${EXISTING}`)
  })

  it('reports a conflict when the import was already resolved elsewhere', async () => {
    mockApi({
      ...base,
      [`GET /imports/${IMPORT_ID}`]: importRecord('duplicate'),
      [`POST /imports/${IMPORT_ID}/resolve`]: () => apiError(409, 'already_resolved', 'This import has already been processed.'),
    })
    const { wrapper } = await mountView(ImportDetailView, { path: `/imports/${IMPORT_ID}`, pattern: '/imports/:id' })
    await wrapper.get('[data-testid="resolve-form"]').trigger('submit')
    await settle()
    expect(useToast().toasts.map((t) => t.message)).toContain('This import has already been processed.')
  })

  it('hides the decision for users who cannot edit products', async () => {
    mockApi({ ...base, [`GET /imports/${IMPORT_ID}`]: importRecord('duplicate') })
    const { wrapper } = await mountView(ImportDetailView, { path: `/imports/${IMPORT_ID}`, pattern: '/imports/:id', permissions: ['products.read'] })
    expect(wrapper.find('[data-testid="resolve-form"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('waiting for a decision')
  })
})
