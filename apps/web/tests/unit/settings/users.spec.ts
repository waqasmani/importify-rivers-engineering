import { afterEach, describe, expect, it, vi } from 'vitest'
import UsersView from '@/views/settings/UsersView.vue'
import { fakeMe, mockApi, mountView, settle } from '../helpers'

const me = fakeMe().user
const ADMIN_ROLE = me.role_id
const OTHER = '33333333-3333-4333-8333-333333333333'
const LOCKED = '44444444-4444-4444-8444-444444444444'
const user = (id: string, over: object = {}) => ({
  id, email: `${id.slice(0, 4)}@example.com`, display_name: `User ${id.slice(0, 4)}`, role_id: ADMIN_ROLE, role_key: 'admin', role_name: 'Administrator',
  is_owner: false, is_active: true, last_login_at: null, locked_until: null, created_at: '2026-01-01T00:00:00Z', ...over,
})
const roles = { items: [{ id: ADMIN_ROLE, key: 'admin', name: 'Administrator', description: '', is_system: true, permissions: [], user_count: 2, created_at: '', updated_at: '' }] }

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

describe('UsersView', () => {
  it('transfers ownership only after the target e-mail is typed', async () => {
    const api = mockApi({
      'GET /users': { items: [{ ...me }, user(OTHER)], total: 2, limit: 50, offset: 0 },
      'GET /roles': roles,
      'GET /auth/me': fakeMe(),
      [`POST /users/${OTHER}/transfer-ownership`]: () => undefined,
    })
    const { wrapper } = await mountView(UsersView)
    expect(wrapper.find(`[data-testid="row-${me.id}"]`).text()).toContain('Owner')
    await wrapper.find(`button[aria-label="Transfer ownership to User 3333"]`).trigger('click')
    await settle(1)
    const submit = () => wrapper.findAll('button').find((b) => b.text() === 'Transfer ownership')!
    expect(submit().attributes('disabled')).toBeDefined()
    await wrapper.find('form#transfer-form input').setValue('wrong@example.com')
    expect(submit().attributes('disabled')).toBeDefined()
    await wrapper.find('form#transfer-form input').setValue('3333@example.com')
    expect(submit().attributes('disabled')).toBeUndefined()
    await wrapper.find('form#transfer-form').trigger('submit')
    await settle()
    expect(api.calls.some((c) => c.method === 'POST' && c.path === `/users/${OTHER}/transfer-ownership`)).toBe(true)
  })

  it('offers no ownership transfer to a non-owner and unlocks with a password reset', async () => {
    const until = new Date(Date.now() + 10 * 60_000).toISOString()
    const notOwner = { ...fakeMe(), user: { ...me, is_owner: false } }
    const api = mockApi({
      'GET /users': { items: [user(OTHER, { is_owner: true }), user(LOCKED, { locked_until: until, role_key: 'member' })], total: 2, limit: 50, offset: 0 },
      'GET /roles': roles,
      [`POST /users/${LOCKED}/password`]: () => undefined,
    })
    const { wrapper, auth } = await mountView(UsersView)
    auth.apply(notOwner)
    await settle(1)
    expect(wrapper.find('button[aria-label^="Transfer ownership"]').exists()).toBe(false)
    // The owner's account cannot be edited or deleted by another administrator.
    expect(wrapper.find('button[aria-label="Edit User 3333"]').exists()).toBe(false)
    expect(wrapper.find('button[aria-label="Delete User 3333"]').exists()).toBe(false)
    expect(wrapper.find(`[data-testid="row-${LOCKED}"]`).text()).toContain('locked until')
    await wrapper.find('button[aria-label="Unlock User 4444"]').trigger('click')
    await settle(1)
    await wrapper.findAll('button').find((b) => b.text().includes('Generate'))!.trigger('click')
    await wrapper.find('form#user-reset-form').trigger('submit')
    await settle()
    const post = api.calls.find((c) => c.method === 'POST')!
    expect(post.path).toBe(`/users/${LOCKED}/password`)
    expect((post.body as { password: string }).password.length).toBeGreaterThanOrEqual(12)
  })
})
