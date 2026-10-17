import { afterEach, describe, expect, it, vi } from 'vitest'
import TokensPanel from '@/components/settings/TokensPanel.vue'
import { mockApi, mountView, settle } from '../helpers'

const TOKEN = 'rea_pat_plaintext_secret_value_0123456789abcdef'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

describe('TokensPanel', () => {
  it('shows a new token exactly once and only the prefix afterwards', async () => {
    const items: object[] = []
    const api = mockApi({
      'GET /auth/tokens': () => ({ items }),
      'POST /auth/tokens': (c) => {
        const cred = { id: 't1', name: (c.body as { name: string }).name, prefix: 'rea_pat_plai', scopes: (c.body as { scopes: string[] }).scopes, expires_at: '2027-01-01T00:00:00Z', last_used_at: null, created_at: '2026-10-01T00:00:00Z' }
        items.push(cred)
        return { token: TOKEN, credential: cred }
      },
    })
    const { wrapper, queryClient } = await mountView(TokensPanel, { permissions: ['crm.read', 'crm.write', 'users.manage', 'backups.manage'] })
    expect(wrapper.text()).toContain('No API tokens')
    await wrapper.findAll('button').find((b) => b.text().includes('New token'))!.trigger('click')
    await settle()
    // Permissions that can never be granted to a token are not offered.
    const offered = wrapper.findAll('input[type="checkbox"]').map((i) => (i.element as HTMLInputElement).value)
    expect(offered).toEqual(['crm.read', 'crm.write'])
    await wrapper.find('form#token-form input').setValue('Nightly sync')
    await wrapper.find('form#token-form').trigger('submit')
    await settle()
    const post = api.calls.find((c) => c.method === 'POST')!
    expect(post.body).toEqual({ name: 'Nightly sync', expires_in_days: 90, scopes: ['crm.read'] })

    const shown = wrapper.find('[data-testid="new-token"]')
    expect(shown.exists()).toBe(true)
    expect((shown.find('[data-testid="secret-value"]').element as HTMLInputElement).value).toBe(TOKEN)
    expect(shown.text()).toContain('shown only once')
    // The plaintext is not kept in the query cache.
    expect(JSON.stringify(queryClient.getQueryCache().getAll().map((q) => q.state.data))).not.toContain(TOKEN)

    await shown.findAll('button').find((b) => b.text().includes('I have copied'))!.trigger('click')
    await settle()
    expect(wrapper.find('[data-testid="new-token"]').exists()).toBe(false)
    expect(document.body.innerHTML).not.toContain(TOKEN)
    expect(wrapper.find('[data-testid="token-t1"]').text()).toContain('rea_pat_plai')
  })
})
