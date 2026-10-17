import { afterEach, describe, expect, it, vi } from 'vitest'
import MarketplacesView from '@/views/products/MarketplacesView.vue'
import { confirmOk, withConfirm } from '../crm/fixtures'
import { mockApi, mountView, settle } from '../helpers'
import { BUILTINS, customRules, shippedDefinition } from './fixtures'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

const amazon = shippedDefinition('amazon')
const custom = customRules()
const all = [...BUILTINS, amazon, shippedDefinition('aliexpress'), shippedDefinition('lazada', { enabled: false, priority: 75 }), custom]

describe('MarketplacesView', () => {
  it('groups shipped definitions by region with domains, version, verification status and extracted fields', async () => {
    mockApi({ 'GET /adapters': { items: all } })
    const { wrapper } = await mountView(MarketplacesView, { path: '/marketplaces' })
    expect(wrapper.find('[data-testid="verification-notice"]').text()).toMatch(/only been checked against synthetic test pages/)
    expect(wrapper.text()).toContain('3 marketplace definitions, 2 enabled.')
    expect(wrapper.find('[data-testid="region-global"]').text()).toContain('Global marketplaces')
    const row = wrapper.find('[data-testid="marketplace-amazon"]')
    expect(row.text()).toContain('Amazon')
    expect(row.text()).toContain('definition v1')
    expect(row.text()).toContain('Synthetic fixture only')
    expect(row.text()).toContain('amazon.co.uk')
    expect(row.text()).not.toContain('*.amazon.co.uk')
    expect(row.text()).toMatch(/Extracts:\s*Title\s*Description\s*Brand\s*SKU\s*Price\s*List price/)
    expect(row.text()).toContain('Variants')
    expect(row.text()).toContain('Assumed page layout')
    expect(row.find('a[href="/adapter-lab?adapter=amazon"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="region-cross_border"]').text()).toContain('AliExpress')
    expect(wrapper.find('[data-testid="marketplace-lazada"]').text()).toContain('disabled')
    expect(wrapper.find('[data-testid="custom-acme-rules"]').text()).toContain('Acme rules')
    // Built-in code adapters are not marketplaces.
    expect(wrapper.text()).not.toContain('Schema.org JSON-LD')
  })

  it('enables, disables and re-prioritises definitions without changing them', async () => {
    const api = mockApi({ 'GET /adapters': { items: all }, [`PUT /adapters/${amazon.id}`]: (c) => ({ ...amazon, ...(c.body as object) }) })
    const { wrapper } = await mountView(MarketplacesView, { path: '/marketplaces' })
    const row = () => wrapper.find('[data-testid="marketplace-amazon"]')
    await row().findAll('button').find((b) => b.text() === 'Disable')!.trigger('click')
    await settle()
    const puts = () => api.calls.filter((c) => c.method === 'PUT')
    expect(puts()[0]!.body).toMatchObject({ key: 'amazon', name: 'Amazon', enabled: false, priority: 60, kind: 'rules', domain_patterns: amazon.domain_patterns, selectors: {} })
    expect((puts()[0]!.body as { rules: unknown }).rules).toEqual(amazon.rules)

    await row().find('input[aria-label="Priority of Amazon"]').setValue('15')
    await row().findAll('button').find((b) => b.text() === 'Save priority')!.trigger('click')
    await settle()
    expect(puts()[1]!.body).toMatchObject({ key: 'amazon', priority: 15, enabled: true })
  })

  it('shows but does not offer changes to users without settings.manage', async () => {
    mockApi({ 'GET /adapters': { items: all } })
    const { wrapper } = await mountView(MarketplacesView, { path: '/marketplaces', permissions: ['products.read'] })
    const row = wrapper.find('[data-testid="marketplace-amazon"]')
    expect(row.findAll('button')).toHaveLength(0)
    expect(row.findAll('input')).toHaveLength(0)
    expect(row.text()).toContain('priority 60')
    expect(wrapper.text()).not.toContain('New rules adapter')
    expect(wrapper.find('[data-testid="custom-acme-rules"]').findAll('button')).toHaveLength(0)
  })

  it('filters by name or domain', async () => {
    mockApi({ 'GET /adapters': { items: all } })
    const { wrapper } = await mountView(MarketplacesView, { path: '/marketplaces' })
    await wrapper.find('input[aria-label="Filter marketplaces"]').setValue('lazada.co.th')
    expect(wrapper.find('[data-testid="marketplace-lazada"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="marketplace-amazon"]').exists()).toBe(false)
  })

  it('deletes custom rules adapters only after confirmation', async () => {
    const api = mockApi({ 'GET /adapters': { items: all }, [`DELETE /adapters/${custom.id}`]: () => undefined })
    const { wrapper } = await mountView(withConfirm(MarketplacesView), { path: '/marketplaces' })
    await wrapper.find('button[aria-label="Delete Acme rules"]').trigger('click')
    await settle()
    expect(api.calls.some((c) => c.method === 'DELETE')).toBe(false)
    confirmOk()
    await settle()
    expect(api.calls.filter((c) => c.method === 'DELETE').map((c) => c.path)).toEqual([`/adapters/${custom.id}`])
  })
})
