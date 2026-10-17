import { afterEach, describe, expect, it, vi } from 'vitest'
import UsersView from '@/views/settings/UsersView.vue'
import { apiError, fakeMe, mockApi, mountView, settle } from '../helpers'

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

  it('offers no ownership transfer to a non-owner and unlocks a locked account', async () => {
    const until = new Date(Date.now() + 10 * 60_000).toISOString()
    const notOwner = { ...fakeMe(), user: { ...me, is_owner: false } }
    const api = mockApi({
      'GET /users': { items: [user(OTHER, { is_owner: true, locked_until: until }), user(LOCKED, { locked_until: until, role_key: 'member' })], total: 2, limit: 50, offset: 0 },
      'GET /roles': roles,
      [`POST /users/${LOCKED}/unlock`]: () => undefined,
    })
    const { wrapper, auth } = await mountView(UsersView)
    auth.apply(notOwner)
    await settle(1)
    expect(wrapper.find('button[aria-label^="Transfer ownership"]').exists()).toBe(false)
    // The owner's account cannot be edited, deleted or unlocked by another administrator.
    expect(wrapper.find('button[aria-label="Edit User 3333"]').exists()).toBe(false)
    expect(wrapper.find('button[aria-label="Delete User 3333"]').exists()).toBe(false)
    expect(wrapper.find('button[aria-label="Unlock User 3333"]').exists()).toBe(false)
    expect(wrapper.find(`[data-testid="row-${LOCKED}"]`).text()).toContain('locked until')
    await wrapper.find('button[aria-label="Unlock User 4444"]').trigger('click')
    await settle()
    const post = api.calls.find((c) => c.method === 'POST')!
    expect(post.path).toBe(`/users/${LOCKED}/unlock`)
    expect(wrapper.find('form#user-reset-form').exists()).toBe(false)
    expect(api.calls.filter((c) => c.method === 'GET' && c.path === '/users')).toHaveLength(2)
  })

  it('searches users on the server and keeps the search in the URL', async () => {
    const api = mockApi({
      'GET /users': (c) => (c.query.get('q') ? { items: [user(OTHER)], total: 1, limit: 50, offset: 0 } : { items: [{ ...me }, user(OTHER)], total: 2, limit: 50, offset: 0 }),
      'GET /roles': roles,
    })
    const { wrapper, router } = await mountView(UsersView, { path: '/settings/users?offset=50', pattern: '/settings/users' })
    expect(api.calls.find((c) => c.path === '/users')!.query.get('offset')).toBe('50')
    await wrapper.find('input#user-search').setValue('3333@exa')
    await new Promise((r) => setTimeout(r, 300))
    await settle()
    expect(router.currentRoute.value.query).toEqual({ q: '3333@exa' })
    const last = api.calls.filter((c) => c.path === '/users').at(-1)!
    expect(last.query.get('q')).toBe('3333@exa')
    expect(last.query.get('offset')).toBeNull()
    expect(wrapper.findAll('tbody tr')).toHaveLength(1)
    expect(wrapper.text()).not.toContain('Searching the first')
  })

  it('changes the e-mail address and shows a duplicate address on the field', async () => {
    let attempt = 0
    const api = mockApi({
      'GET /users': { items: [{ ...me }, user(OTHER)], total: 2, limit: 50, offset: 0 },
      'GET /roles': roles,
      [`PUT /users/${OTHER}`]: (c) => (++attempt === 1 ? apiError(409, 'duplicate', 'A user with this e-mail address already exists.') : { ...user(OTHER), ...(c.body as object) }),
    })
    const { wrapper } = await mountView(UsersView)
    await wrapper.find('button[aria-label="Edit User 3333"]').trigger('click')
    await settle(1)
    const email = wrapper.find('form#user-edit-form input[type="email"]')
    expect((email.element as HTMLInputElement).value).toBe('3333@example.com')
    await email.setValue(' owner@example.com ')
    await wrapper.find('form#user-edit-form').trigger('submit')
    await settle()
    const put = api.calls.find((c) => c.method === 'PUT')!
    expect(put.body).toMatchObject({ email: 'owner@example.com', display_name: 'User 3333', role_id: ADMIN_ROLE, is_active: true })
    expect(wrapper.find('form#user-edit-form').text()).toContain('A user with this e-mail address already exists.')
    await email.setValue('new@example.com')
    await wrapper.find('form#user-edit-form').trigger('submit')
    await settle()
    expect(api.calls.filter((c) => c.method === 'PUT').at(-1)!.body).toMatchObject({ email: 'new@example.com' })
    expect(wrapper.find('form#user-edit-form').exists()).toBe(false)
  })
})
