import { afterEach, describe, expect, it, vi } from 'vitest'
import AdapterLabView from '@/views/products/AdapterLabView.vue'
import { runLab } from '@/components/marketplaces/lab'
import { toAdapterConfig, type AdapterConfig } from '@rea/extractor'
import { mockApi, mountView, settle } from '../helpers'
import { BUILTINS, customRules, marketplaceFixture, shippedDefinition } from './fixtures'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

const aliexpress = shippedDefinition('aliexpress')
const amazon = shippedDefinition('amazon', { enabled: false })
const all = [...BUILTINS, aliexpress, amazon, customRules()]
const configs: AdapterConfig[] = all.flatMap((a) => {
  const r = toAdapterConfig(a)
  return r.ok ? [r.cfg] : []
})
const ALI_URL = 'https://www.aliexpress.com/item/1005001234567890.html'

describe('runLab', () => {
  const html = marketplaceFixture('aliexpress')

  it('runs the enabled adapters on an inert copy of the page', () => {
    const before = document.body.innerHTML
    const r = runLab({ html, url: ALI_URL, mode: 'all', fallbacks: true }, configs)
    if (!r.ok) throw new Error(r.error)
    expect(r.run.page.product.payload.title).toBe('Folding LED Desk Lamp, 3 Colour Modes')
    expect(r.run.page.product.payload.adapter_key).toBe('aliexpress')
    expect(r.run.outputs.map((o) => o.cfg.key)).not.toContain('amazon')
    // The saved page never reaches the live document and its scripts never run.
    expect(document.body.innerHTML).toBe(before)
    expect(document.querySelectorAll('script, img').length).toBe(0)
    expect((window as unknown as Record<string, unknown>)['runParams']).toBeUndefined()
  })

  it('tests one adapter, even a disabled one or one outside its domains, and says so', () => {
    const r = runLab({ html, url: 'https://mirror.example/item/1.html', mode: 'one', adapterKey: 'aliexpress', fallbacks: false }, configs)
    if (!r.ok) throw new Error(r.error)
    expect(r.run.outputs.map((o) => o.cfg.key)).toEqual(['aliexpress'])
    expect(r.run.focus?.output.fields.title?.value).toBe('Folding LED Desk Lamp, 3 Colour Modes')
    expect(r.run.notes.join(' ')).toMatch(/mirror.example is not one of AliExpress's domains/)
    const off = runLab({ html: marketplaceFixture('amazon'), url: 'https://www.amazon.com/dp/B0TESTHAMK', mode: 'one', adapterKey: 'amazon', fallbacks: true }, configs)
    if (!off.ok) throw new Error(off.error)
    expect(off.run.notes.join(' ')).toMatch(/Amazon is disabled/)
    expect(off.run.page.product.payload.price_amount).toBe('29.99')
  })

  it('runs draft rule documents and reports their problems', () => {
    const draft = JSON.stringify({ schema: 1, product_signal: { any: [{ css: 'h1' }] }, fields: { title: [{ css: 'h1[data-pl]' }], price: [{ css: '.product-price-current' }] } })
    const r = runLab({ html, url: ALI_URL, mode: 'draft', draft, fallbacks: false }, configs)
    if (!r.ok) throw new Error(r.error)
    expect(r.run.focus?.cfg.key).toBe('draft')
    expect(r.run.page.product.payload.price_amount).toBe('18.40')
    expect(r.run.page.product.payload.evidence['price']).toBe('draft: .product-price-current (rule price[0])')
    const bad = runLab({ html, url: ALI_URL, mode: 'draft', draft: '{"schema": 1}', fallbacks: false }, configs)
    expect(bad.ok).toBe(false)
    if (!bad.ok) expect(bad.issues?.length).toBeGreaterThan(0)
  })

  it('requires an http(s) address and some HTML', () => {
    expect(runLab({ html, url: 'javascript:alert(1)', mode: 'all', fallbacks: true }, configs)).toMatchObject({ ok: false })
    expect(runLab({ html: '  ', url: ALI_URL, mode: 'all', fallbacks: true }, configs)).toMatchObject({ ok: false })
  })
})

describe('AdapterLabView', () => {
  it('reads a pasted page and shows fields with confidence and evidence, images as links only', async () => {
    mockApi({ 'GET /adapters': { items: all } })
    const { wrapper } = await mountView(AdapterLabView, { path: '/adapter-lab' })
    await wrapper.find('[data-testid="lab-url"]').setValue(ALI_URL)
    await wrapper.find('[data-testid="lab-html"]').setValue(marketplaceFixture('aliexpress'))
    await wrapper.find('form').trigger('submit')
    await settle()
    const fields = wrapper.find('[data-testid="lab-fields"]')
    expect(fields.find('[data-testid="lab-field-title"]').text()).toContain('Folding LED Desk Lamp, 3 Colour Modes')
    expect(fields.find('[data-testid="lab-field-title"]').text()).toContain('97%')
    expect(fields.find('[data-testid="lab-field-title"]').text()).toContain('aliexpress: script "window.runParams" → data.productInfoComponent.subject (rule title[0])')
    expect(fields.find('[data-testid="lab-field-price"]').text()).toContain('18.4')
    expect(wrapper.find('[data-testid="lab-variants"]').text()).toContain('Ships From: United States')
    expect(wrapper.text()).toContain('Offer metadata')
    expect(wrapper.find('[data-testid="lab-page-type"]').text()).toContain('product')
    // Images are listed as text links; nothing from the saved page is rendered or loaded.
    expect(wrapper.findAll('img')).toHaveLength(0)
    expect(document.querySelectorAll('img, script, iframe').length).toBe(0)
    expect(wrapper.find('a[href="https://ae01.alicdn.example/kf/Slamp1.jpg"]').attributes('rel')).toBe('noopener noreferrer nofollow')
    expect(wrapper.html()).not.toContain('runparams-csrf-should-not-leak')
  })

  it('preselects the adapter from a "Test in lab" link and shows what it found', async () => {
    mockApi({ 'GET /adapters': { items: all } })
    const { wrapper } = await mountView(AdapterLabView, { path: '/adapter-lab?adapter=aliexpress' })
    expect((wrapper.find('[data-testid="lab-adapter"]').element as HTMLSelectElement).value).toBe('aliexpress')
    await wrapper.find('[data-testid="lab-url"]').setValue('https://mirror.example/x.html')
    await wrapper.find('[data-testid="lab-html"]').setValue(marketplaceFixture('aliexpress'))
    await wrapper.find('form').trigger('submit')
    await settle()
    expect(wrapper.text()).toContain('What AliExpress found')
    expect(wrapper.find('[data-testid="lab-focus-table"]').text()).toContain('data.priceComponent.discountPrice.minActivityAmount')
    expect(wrapper.text()).toMatch(/is not one of AliExpress's domains/)
  })

  it('validates draft rule documents as you type and offers to save them', async () => {
    mockApi({ 'GET /adapters': { items: all } })
    const { wrapper } = await mountView(AdapterLabView, { path: '/adapter-lab' })
    await wrapper.find('[data-testid="lab-mode-draft"]').setValue(true)
    await wrapper.find('[data-testid="lab-draft"]').setValue('{"schema": 1, "fields": {"title": [{"css": "h1", "attr": "value"}]}}')
    const issues = wrapper.find('[data-testid="lab-draft-issues"]').text()
    expect(issues).toContain('product_signal')
    expect(issues).toContain('form values, event handlers and token-like attributes cannot be read')
    expect(wrapper.text()).not.toContain('Save draft as a rules adapter')
    await wrapper.find('[data-testid="lab-draft"]').setValue('{"schema": 1, "product_signal": {"any": [{"css": "h1"}]}, "fields": {"title": [{"css": "h1"}]}}')
    expect(wrapper.find('[data-testid="lab-draft-valid"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('Save draft as a rules adapter')
  })

  it('reports missing input instead of reading', async () => {
    mockApi({ 'GET /adapters': { items: all } })
    const { wrapper } = await mountView(AdapterLabView, { path: '/adapter-lab' })
    await wrapper.find('[data-testid="lab-url"]').setValue(ALI_URL)
    await wrapper.find('form').trigger('submit')
    await settle()
    expect(wrapper.find('[data-testid="lab-error"]').text()).toContain('Paste the page HTML')
  })
})
