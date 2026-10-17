import { afterEach, describe, expect, it, vi } from 'vitest'
import RulesAdapterFormModal from '@/components/marketplaces/RulesAdapterFormModal.vue'
import { apiError, mockApi, mountView, settle } from '../helpers'
import { CUSTOM_RULES, customRules } from './fixtures'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

describe('RulesAdapterFormModal', () => {
  it('validates the rule document as you type and creates a rules adapter', async () => {
    const api = mockApi({ 'POST /adapters': (c) => customRules(c.body as object) })
    const { wrapper } = await mountView(RulesAdapterFormModal, { props: { open: true, adapter: null } })
    expect(wrapper.find('[data-testid="rules-valid"]').text()).toContain('It can fill: Title, Price, Currency, Images')
    await wrapper.find('input.input').setValue('Northwind Shop')
    const json = wrapper.find('[data-testid="rules-json"]')
    await json.setValue('{"schema": 1, "product_signal": {"any": [{"css": "h1"}]}, "fields": {"title": [{"source": "state", "path": "csrfToken"}]}}')
    const issues = wrapper.find('[data-testid="rules-issues"]').text()
    expect(issues).toContain('looks like a credential or token name')
    expect(issues).toContain('unknown source "state"')
    expect(wrapper.find('button[type="submit"]').attributes('disabled')).toBeDefined()
    await json.setValue(JSON.stringify(CUSTOM_RULES))
    const textareas = wrapper.findAll('textarea')
    await textareas[1]!.setValue('northwind.example\n*.northwind.example')
    await wrapper.find('form').trigger('submit')
    await settle()
    const post = api.calls.find((c) => c.method === 'POST')!
    expect(post.body).toEqual({
      key: 'northwind-shop', name: 'Northwind Shop', description: '', domain_patterns: ['northwind.example', '*.northwind.example'], priority: 50,
      enabled: true, selectors: {}, kind: 'rules', rules: CUSTOM_RULES,
    })
  })

  it('shows the server validation next to the rule document', async () => {
    const existing = customRules()
    mockApi({ [`PUT /adapters/${existing.id}`]: () => apiError(422, 'validation_failed', 'Some fields are invalid.', { rules: 'fields.title[0].css: CSS selectors cannot contain <, >, { or }' }) })
    const { wrapper } = await mountView(RulesAdapterFormModal, { props: { open: true, adapter: existing } })
    expect((wrapper.find('[data-testid="rules-json"]').element as HTMLTextAreaElement).value).toContain('"h1.pdp"')
    expect(wrapper.find('input.font-mono').attributes('disabled')).toBeDefined()
    await wrapper.find('form').trigger('submit')
    await settle()
    expect(wrapper.text()).toContain('fields.title[0].css: CSS selectors cannot contain <, >, { or }')
  })

  it('prefills drafts from the Adapter lab', async () => {
    const draft = JSON.stringify(CUSTOM_RULES)
    const { wrapper } = await mountView(RulesAdapterFormModal, { props: { open: true, adapter: null, draft: { rules: draft, domains: ['acme.example', '*.acme.example'] } } })
    expect((wrapper.find('[data-testid="rules-json"]').element as HTMLTextAreaElement).value).toBe(draft)
    expect((wrapper.findAll('textarea')[1]!.element as HTMLTextAreaElement).value).toBe('acme.example\n*.acme.example')
  })
})
