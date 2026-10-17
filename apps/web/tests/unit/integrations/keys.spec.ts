import { afterEach, describe, expect, it, vi } from 'vitest'
import IntegrationKeysPanel from '@/components/integrations/IntegrationKeysPanel.vue'
import { apiError, mockApi, mountView, settle } from '../helpers'
import { withConfirm, confirmOk } from '../crm/fixtures'
import { config, key } from './fixtures'

const TOKEN = 'reai_plaintext_integration_key_0123456789abcdefghij'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

const mount = () => mountView(withConfirm(IntegrationKeysPanel), { props: { scopes: config.scopes }, permissions: ['settings.manage'] })

describe('IntegrationKeysPanel', () => {
  it('creates a key with scopes and shows the token exactly once', async () => {
    const items: object[] = []
    const api = mockApi({
      'GET /integrations/keys': () => ({ items }),
      'POST /integrations/keys': (c) => {
        const body = c.body as { name: string; scopes: string[] }
        const k = key({ name: body.name, scopes: body.scopes })
        items.push(k)
        return { token: TOKEN, key: k }
      },
    })
    const { wrapper, queryClient } = await mount()
    expect(wrapper.text()).toContain('No integration keys')
    await wrapper.findAll('button').find((b) => b.text().includes('New key'))!.trigger('click')
    await settle()
    await wrapper.find('form#key-form input').setValue('ERP sync')
    // Choosing costs also selects catalog.read, which it requires.
    const costs = wrapper.find('input[value="catalog.read_costs"]')
    ;(costs.element as HTMLInputElement).checked = true
    await costs.trigger('change')
    await wrapper.find('form#key-form select').setValue('90')
    await wrapper.find('form#key-form').trigger('submit')
    await settle()
    expect(api.calls.find((c) => c.method === 'POST')!.body).toEqual({ name: 'ERP sync', description: '', scopes: ['catalog.read', 'catalog.read_costs'], expires_in_days: 90 })

    const shown = wrapper.find('[data-testid="new-key"]')
    expect((shown.find('[data-testid="secret-value"]').element as HTMLInputElement).value).toBe(TOKEN)
    expect(JSON.stringify(queryClient.getQueryCache().getAll().map((q) => q.state.data))).not.toContain(TOKEN)
    await shown.findAll('button').find((b) => b.text().includes('I have copied'))!.trigger('click')
    await settle()
    expect(document.body.innerHTML).not.toContain(TOKEN)
    expect(wrapper.find('[data-testid="key-k1"]').text()).toContain('reai_AbCdEfG')
    expect(wrapper.find('[data-testid="key-k1"]').text()).toContain('catalog.read_costs')
  })

  it('shows server validation next to the scopes', async () => {
    mockApi({
      'GET /integrations/keys': { items: [] },
      'POST /integrations/keys': () => apiError(422, 'validation_failed', 'Some fields are invalid.', { scopes: 'catalog.read_costs requires catalog.read' }),
    })
    const { wrapper } = await mount()
    await wrapper.findAll('button').find((b) => b.text().includes('New key'))!.trigger('click')
    await settle()
    await wrapper.find('form#key-form input').setValue('X')
    await wrapper.find('form#key-form').trigger('submit')
    await settle()
    expect(wrapper.text()).toContain('catalog.read_costs requires catalog.read')
    expect(wrapper.find('[data-testid="new-key"]').exists()).toBe(false)
  })

  it('lists usage and revokes only after confirmation', async () => {
    let items = [key({ last_used_at: '2026-10-16T10:00:00Z', last_used_ip: '203.0.113.9', requests_today: 12, requests_30d: 340, creator_removed: true, created_by: 'gone@example.com' }),
      key({ id: 'k2', name: 'Old', status: 'revoked', revoked_at: '2026-10-02T00:00:00Z' })]
    const api = mockApi({
      'GET /integrations/keys': () => ({ items }),
      'DELETE /integrations/keys/k1': () => {
        items = items.map((k) => (k.id === 'k1' ? { ...k, status: 'revoked' } : k))
        return undefined
      },
    })
    const { wrapper } = await mount()
    const row = wrapper.find('[data-testid="key-k1"]')
    expect(row.text()).toContain('203.0.113.9')
    expect(row.text()).toContain('12 requests today, 340 in 30 days')
    expect(row.text()).toContain('(user removed)')
    expect(wrapper.find('[data-testid="key-k2"]').exists()).toBe(false)
    await row.findAll('button').find((b) => b.text().includes('Revoke'))!.trigger('click')
    await settle()
    expect(api.calls.some((c) => c.method === 'DELETE')).toBe(false)
    expect(document.body.textContent).toContain('lose')
    confirmOk()
    await settle()
    expect(api.calls.some((c) => c.method === 'DELETE' && c.path === '/integrations/keys/k1')).toBe(true)
    await wrapper.find('input[type="checkbox"]').setValue(true)
    await settle()
    expect(wrapper.find('[data-testid="key-k2"]').text()).toContain('revoked')
  })
})
