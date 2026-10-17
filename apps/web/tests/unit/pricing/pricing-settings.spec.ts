import { afterEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import PricingSettingsView from '@/views/settings/PricingSettingsView.vue'
import ConfirmHost from '@/components/ui/ConfirmHost.vue'
import { apiError, mockApi, mountView, settle, type Call } from '../helpers'
import { RULE_A, RULE_B, previewDoc, ratesDoc, rule, rulesDoc, tagsDoc } from './fixtures'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

const Host = defineComponent({ render: () => h('div', [h(PricingSettingsView), h(ConfirmHost)]) })

function routes(extra: Record<string, unknown> = {}) {
  return {
    'GET /pricing/rules': rulesDoc(),
    'GET /pricing/rates': ratesDoc(),
    'GET /tags': tagsDoc(),
    'GET /settings': { entity_labels: {}, imports: { default_currency: 'USD' } },
    'POST /pricing/preview': (c: Call) => previewDoc({ cost: (c.body as { cost: string }).cost }),
    'POST /pricing/rules/reorder': (c: Call) => {
      const ids = (c.body as { ids: string[] }).ids
      const doc = rulesDoc()
      const byId = Object.fromEntries(doc.items.map((r) => [r.id, r]))
      return { ...doc, items: [...ids.map((id) => byId[id]!), doc.items[2]!] }
    },
    ...extra,
  }
}

const wait = (ms: number) => new Promise((r) => setTimeout(r, ms))

describe('PricingSettingsView', () => {
  it('lists rules in evaluation order with the default rule last and summaries of what they do', async () => {
    mockApi(routes())
    const { wrapper } = await mountView(Host, { path: '/settings/pricing' })
    const list = wrapper.get('[data-testid="rule-list"]')
    const items = list.findAll('li')
    expect(items).toHaveLength(2)
    expect(items[0]!.text()).toContain('AliExpress')
    expect(items[0]!.text()).toContain('from aliexpress.com')
    expect(items[0]!.text()).toContain('ends in .99')
    expect(items[0]!.text()).toContain('3 products')
    expect(items[1]!.text()).toContain('cost below $5.00')
    const def = wrapper.get('[data-testid="default-rule"]')
    expect(def.text()).toContain('Default rule')
    expect(def.text()).toContain('+ 40%')
    expect(def.text()).not.toContain('Delete')
    expect(wrapper.get('[data-testid="currency-rates"]').text()).toContain('1 EUR = 1.0882 USD')
    expect(wrapper.get('[data-testid="stale-rates"]').text()).toContain('1 EUR = 0.85 GBP')
  })

  it('reorders with the arrow buttons, the keyboard and drag and drop', async () => {
    const api = mockApi(routes())
    const { wrapper } = await mountView(Host, { path: '/settings/pricing' })
    await wrapper.get('button[aria-label="Move AliExpress down"]').trigger('click')
    await settle()
    const reorders = () => api.calls.filter((c) => c.path === '/pricing/rules/reorder').map((c) => (c.body as { ids: string[] }).ids)
    expect(reorders()[0]).toEqual([RULE_B, RULE_A])
    expect(wrapper.get('[data-testid="rule-list"]').findAll('li')[0]!.text()).toContain('Cheap items')

    // Keyboard: arrow keys on the handle move the rule and keep focus on it.
    const handle = wrapper.get(`[data-handle="${RULE_A}"]`)
    await handle.trigger('keydown', { key: 'ArrowUp' })
    await settle()
    expect(reorders()[1]).toEqual([RULE_A, RULE_B])
    expect(document.activeElement?.getAttribute('data-handle')).toBe(RULE_A)
    expect(wrapper.text()).toContain('AliExpress moved to position 1 of 2.')

    // Drag the first rule onto the second.
    const items = wrapper.get('[data-testid="rule-list"]').findAll('li')
    await items[0]!.trigger('dragstart', { dataTransfer: { setData: () => undefined, effectAllowed: '' } })
    await items[1]!.trigger('drop')
    await settle()
    expect(reorders()[2]).toEqual([RULE_B, RULE_A])
  })

  it('restores the order when the server refuses it', async () => {
    mockApi(routes({ 'POST /pricing/rules/reorder': () => apiError(422, 'validation_failed', 'Some fields are invalid.', { ids: 'must list every pricing rule' }) }))
    const { wrapper } = await mountView(Host, { path: '/settings/pricing' })
    await wrapper.get('button[aria-label="Move AliExpress down"]').trigger('click')
    await settle()
    expect(wrapper.get('[data-testid="rule-list"]').findAll('li')[0]!.text()).toContain('AliExpress')
  })

  it('creates a rule from the form with a live preview of the draft', async () => {
    const api = mockApi(routes({ 'POST /pricing/rules': (c: Call) => rule({ id: 'new', ...(c.body as object) }) }))
    const { wrapper } = await mountView(Host, { path: '/settings/pricing' })
    await wrapper.findAll('button').find((b) => b.text().includes('New rule'))!.trigger('click')
    await settle()
    const form = wrapper.get('[data-testid="pricing-rule-form"]')
    await form.get('[data-testid="rule-name"]').setValue('Kitchen')
    await form.get('[data-testid="rule-domains"]').setValue('Shop.Example.com\nalibaba.com')
    await form.get('[data-testid="rule-markup"]').setValue('60')
    await form.get('[data-testid="rule-fixed"]').setValue('2.5')
    await form.get('[data-testid="rule-min-margin"]').setValue('25')
    await form.get('[data-testid="rule-rounding"]').setValue('ending')
    await form.get('[data-testid="rule-ending"]').setValue('0.95')
    await form.get('[data-testid="rule-compare-mode"]').setValue('percent')
    await form.get('[data-testid="rule-compare-value"]').setValue('30')
    await wait(350)
    await settle()
    const previews = api.calls.filter((c) => c.path === '/pricing/preview')
    const last = previews[previews.length - 1]!.body as { rule: Record<string, unknown>; cost: string }
    expect(last.cost).toBe('10')
    expect(last.rule).toMatchObject({ name: 'Kitchen', markup_percent: '60', rounding: 'ending', rounding_ending: '0.95', compare_at_mode: 'percent', compare_at_value: '30' })
    expect(wrapper.get('[data-testid="pricing-preview"]').text()).toContain('$19.99')

    await form.trigger('submit')
    await settle()
    const post = api.calls.find((c) => c.method === 'POST' && c.path === '/pricing/rules')!
    expect(post.body).toMatchObject({
      name: 'Kitchen', enabled: true, source_domains: ['shop.example.com', 'alibaba.com'], markup_percent: '60', fixed_amount: '2.5', fixed_amount_before_markup: false,
      min_margin_percent: '25', rounding: 'ending', rounding_ending: '0.95', rounding_step: null, compare_at_mode: 'percent', compare_at_value: '30', supplier_ids: [], tag_ids: [],
    })
  })

  it('edits the default rule without conditions and shows server errors next to fields', async () => {
    const api = mockApi(routes({ 'PUT /pricing/rules/d3333333-3333-4333-8333-333333333333': () => apiError(422, 'validation_failed', 'Some fields are invalid.', { markup_percent: 'must be at most 10000' }) }))
    const { wrapper } = await mountView(Host, { path: '/settings/pricing' })
    await wrapper.get('button[aria-label="Edit Default rule"]').trigger('click')
    await settle()
    const form = wrapper.get('[data-testid="pricing-rule-form"]')
    expect(form.find('[data-testid="rule-conditions"]').exists()).toBe(false)
    expect(form.text()).toContain('cannot be deleted')
    await form.get('[data-testid="rule-markup"]').setValue('20000')
    await form.trigger('submit')
    await settle()
    const put = api.calls.find((c) => c.method === 'PUT')!
    expect(put.body).not.toHaveProperty('source_domains')
    expect(put.body).toMatchObject({ name: 'Default rule', markup_percent: '20000' })
    expect(form.text()).toContain('must be at most 10000')
  })

  it('toggles and deletes rules, the deletion after a danger confirmation', async () => {
    const api = mockApi(routes({ [`PUT /pricing/rules/${RULE_A}`]: (c: Call) => rule(c.body as object), [`DELETE /pricing/rules/${RULE_B}`]: () => undefined }))
    const { wrapper } = await mountView(Host, { path: '/settings/pricing' })
    const first = wrapper.get(`[data-testid="rule-${RULE_A}"]`)
    await first.findAll('button').find((b) => b.text() === 'Disable')!.trigger('click')
    await settle()
    expect(api.calls.find((c) => c.method === 'PUT')!.body).toMatchObject({ enabled: false, name: 'AliExpress', multiplier: '2', source_domains: ['aliexpress.com'] })

    await wrapper.get('button[aria-label="Delete Cheap items"]').trigger('click')
    await settle()
    expect(document.body.textContent).toContain('No stored product was priced by it.')
    expect(api.calls.some((c) => c.method === 'DELETE')).toBe(false)
    ;(document.querySelector('[data-testid="confirm-ok"]') as HTMLButtonElement).click()
    await settle()
    expect(api.calls.some((c) => c.method === 'DELETE' && c.path === `/pricing/rules/${RULE_B}`)).toBe(true)
  })

  it('previews the saved rules and maintains exchange rates', async () => {
    const api = mockApi(routes({ 'PUT /pricing/rates/CNY': (c: Call) => ({ ...ratesDoc().items[0], currency: 'CNY', ...(c.body as object) }), 'DELETE /pricing/rates/EUR': () => undefined }))
    const Page = defineComponent({ render: () => h('div', [h(PricingSettingsView), h(ConfirmHost)]) })
    const { wrapper } = await mountView(Page, { path: '/settings/pricing' })
    const calc = wrapper.findAll('[data-testid="pricing-preview"]')[0]!
    await calc.get('[data-testid="preview-cost"]').setValue('12.5')
    await calc.get('[data-testid="preview-currency"]').setValue('eur')
    await calc.get('[data-testid="preview-domain"]').setValue('www.aliexpress.com')
    await wait(350)
    await settle()
    const previews = api.calls.filter((c) => c.path === '/pricing/preview')
    expect(previews[previews.length - 1]!.body).toEqual({ cost: '12.5', currency: 'EUR', source_domain: 'www.aliexpress.com', category: '', supplier_id: null, tag_ids: [], rule: null, rule_id: null })
    expect(calc.get('[data-testid="evaluation-rule"]').text()).toBe('AliExpress')

    const rates = wrapper.get('[data-testid="rate-form"]')
    await rates.get('[data-testid="rate-currency"]').setValue('cny')
    await rates.get('[data-testid="rate-value"]').setValue('0.1389')
    await rates.trigger('submit')
    await settle()
    expect(api.calls.find((c) => c.method === 'PUT')).toMatchObject({ path: '/pricing/rates/CNY', body: { rate: '0.1389', note: '' } })

    await wrapper.get('button[aria-label="Delete the EUR rate"]').trigger('click')
    await settle()
    ;(document.querySelector('[data-testid="confirm-ok"]') as HTMLButtonElement).click()
    await settle()
    expect(api.calls.some((c) => c.method === 'DELETE' && c.path === '/pricing/rates/EUR')).toBe(true)
  })
})
