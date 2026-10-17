import { afterEach, describe, expect, it, vi } from 'vitest'
import NewImportView from '@/views/products/NewImportView.vue'
import CaptureDetailView from '@/views/products/CaptureDetailView.vue'
import { mockApi, mountView, settle } from '../helpers'
import { CAPTURE } from './fixtures'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

const extracted = {
  title: 'Trailblazer 2L Insulated Flask',
  url: 'https://shop.example.com/products/trailblazer?utm_source=x',
  canonical_url: 'https://shop.example.com/products/trailblazer',
  brand: { '@type': 'Brand', name: 'Northwind' },
  sku: 'NW-TB-2L',
  gtin13: '4006381333931',
  price: '€39,95',
  currency: 'eur',
  availability: 'https://schema.org/InStock',
  images: ['https://cdn.example.com/front.jpg', { url: 'https://cdn.example.com/side.jpg', alt: 'Side' }],
  specifications: [{ name: 'Capacity', value: '2 L' }, { name: 'Weight', value: '820 g' }],
  variants: [{ title: 'Forest', options: { Color: 'Forest' } }],
  supplier_name: 'Northwind Outdoor GmbH',
  extraction_method: 'json-ld',
  confidence: 0.92,
  field_confidence: { title: 0.98 },
  evidence: { price: 'offers.price' },
}

function capture(over: object = {}) {
  return {
    capture: {
      id: CAPTURE, url: 'https://shop.example.com/products/trailblazer?utm_source=x', canonical_url: 'https://shop.example.com/products/trailblazer', domain: 'shop.example.com',
      title: 'Trailblazer | Northwind', description: 'A flask.', page_type: 'product', adapter_key: 'json-ld', extraction_method: 'json-ld',
      structured_data: { meta: { generator: 'WordPress' }, jsonld: [{ '@type': 'Product', name: 'Trailblazer 2L Insulated Flask' }] }, extracted,
      selections: [
        { kind: 'text', text: 'Ships in 3 days', selector: '#shipping' },
        { kind: 'table', text: '', selector: 'table.specs', rows: [['Capacity', '2 L']] },
      ],
      warnings: ['the page loads third-party scripts that were not captured'], html_snapshot: null, content_sha256: 'abc', captured_by: null, device_id: null,
      captured_at: '2026-10-01T10:00:00Z', ...over,
    },
    has_snapshot: true,
  }
}

const base = { 'GET /settings': { entity_labels: {}, imports: { default_currency: 'EUR' } }, 'GET /extension/devices': { items: [] }, 'GET /notes': { items: [] } }

describe('Capture → import prefill', () => {
  it('prefills the import form from the captured page and links it as evidence', async () => {
    const api = mockApi({
      ...base,
      [`GET /captures/${CAPTURE}`]: capture(),
      'POST /imports/products/check': (c) => ({ normalized: { ...(c.body as { payload: object }).payload, source_url: null, canonical_url: null, price: '39.95', price_currency: 'EUR', variants: [], images: [], specifications: {}, identifiers: {}, evidence: {}, warnings: [] }, warnings: [], errors: {}, duplicates: [], definite_duplicate: false, suggested_supplier: null }),
    })
    const { wrapper } = await mountView(NewImportView, { path: `/imports/new?capture=${CAPTURE}`, pattern: '/imports/new' })
    expect(wrapper.get('[data-testid="prefill-banner"]').text()).toContain('Trailblazer | Northwind')
    const form = wrapper.get('[data-testid="payload-form"]')
    const value = (sel: string) => (form.find(sel).element as HTMLInputElement).value
    expect((form.findAll('input')[0]!.element as HTMLInputElement).value).toBe('Trailblazer 2L Insulated Flask')
    expect(value('input[inputmode="numeric"]')).toBe('4006381333931')
    expect(value('input[placeholder="EUR"]')).toBe('EUR')
    expect((form.find('select').element as HTMLSelectElement).value).toBe('in_stock')
    expect(form.findAll('[data-testid="image-row"]')).toHaveLength(2)
    expect(form.findAll('[data-testid="variant-row"]')).toHaveLength(1)
    expect(value('input[aria-label="Specification 2"]')).toBe('Weight')
    expect(form.text()).toContain('Extracted with json-ld')

    await wrapper.findAll('form')[0]!.trigger('submit')
    await settle()
    const check = api.calls.find((c) => c.path === '/imports/products/check')!
    expect(check.body).toMatchObject({
      captured_page_id: CAPTURE,
      source: 'manual',
      payload: {
        title: 'Trailblazer 2L Insulated Flask',
        brand: 'Northwind',
        gtin: '4006381333931',
        price: '€39,95',
        currency: 'EUR',
        availability: 'in_stock',
        canonical_url: 'https://shop.example.com/products/trailblazer',
        specifications: { Capacity: '2 L', Weight: '820 g' },
        images: [{ url: 'https://cdn.example.com/front.jpg', alt: '' }, { url: 'https://cdn.example.com/side.jpg', alt: 'Side' }],
        variants: [{ title: 'Forest', options: { Color: 'Forest' } }],
        extraction_method: 'json-ld',
        confidence: 0.92,
        field_confidence: { title: 0.98 },
        evidence: { price: 'offers.price' },
      },
    })
  })

  it('shows an error state when the capture cannot be loaded', async () => {
    mockApi(base)
    const { wrapper } = await mountView(NewImportView, { path: `/imports/new?capture=${CAPTURE}`, pattern: '/imports/new' })
    expect(wrapper.text()).toContain('Could not load the captured page')
  })
})

describe('CaptureDetailView', () => {
  it('offers import and analysis, shows extracted and structured data, and previews the snapshot sandboxed', async () => {
    const api = mockApi({
      ...base,
      [`GET /captures/${CAPTURE}`]: capture(),
      [`GET /captures/${CAPTURE}/snapshot`]: () =>
        new Response('<html><head><link rel="icon" href="https://shop.example.com/icon.png"></head><body><h1>Flask</h1><div style="background: url(https://cdn.example.com/bg.png)">x</div><img src="https://cdn.example.com/a.jpg"><a href="https://shop.example.com/">Home</a></body></html>', {
          status: 200, headers: { 'Content-Type': 'text/plain; charset=utf-8', 'Content-Disposition': 'attachment; filename="capture.html.txt"' },
        }),
    })
    const { wrapper } = await mountView(CaptureDetailView, { path: `/captures/${CAPTURE}`, pattern: '/captures/:id' })
    expect(wrapper.get('[data-testid="import-capture"]').attributes('href')).toBe(`/imports/new?capture=${CAPTURE}`)
    expect(wrapper.get('[data-testid="analyze-capture"]').attributes('href')).toBe(`/analysis/new?capture=${CAPTURE}`)
    expect(wrapper.get('[data-testid="extracted"]').text()).toContain('NW-TB-2L')
    expect(wrapper.get('[data-testid="selections"]').text()).toContain('Ships in 3 days')
    const structured = wrapper.get('[data-testid="structured-data"]')
    const summaries = structured.findAll('summary').map((s) => s.text())
    expect(summaries[0]).toContain('JSON-LD')
    expect(structured.find('details').attributes('open')).toBeDefined()
    expect(structured.find('pre').text()).toContain('"@type": "Product"')
    expect(wrapper.get('[data-testid="capture-warnings"]').text()).toContain('third-party scripts')

    await wrapper.findAll('button').find((b) => b.text() === 'Show preview')!.trigger('click')
    await settle()
    const frame = wrapper.get('[data-testid="snapshot-frame"]')
    expect(frame.attributes('sandbox')).toBe('')
    const doc = frame.attributes('srcdoc')!
    expect(doc).toContain('<h1>Flask</h1>')
    expect(doc).toContain('Content-Security-Policy')
    expect(doc).not.toContain('https://shop.example.com/icon.png')
    expect(doc).not.toContain('https://cdn.example.com/bg.png')
    expect(doc).not.toContain('https://cdn.example.com/a.jpg')
    expect(doc).not.toMatch(/href="https:/)
    expect(api.calls.some((c) => c.path === `/captures/${CAPTURE}/snapshot`)).toBe(true)
  })

  it('hides import, analysis and delete actions without the permissions', async () => {
    mockApi({ ...base, [`GET /captures/${CAPTURE}`]: capture() })
    const { wrapper } = await mountView(CaptureDetailView, { path: `/captures/${CAPTURE}`, pattern: '/captures/:id', permissions: ['products.read'] })
    expect(wrapper.find('[data-testid="import-capture"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="analyze-capture"]').exists()).toBe(false)
    expect(wrapper.findAll('button').map((b) => b.text())).not.toContain('Delete')
  })
})
