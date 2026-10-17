import { afterEach, describe, expect, it, vi } from 'vitest'
import RolesView from '@/views/settings/RolesView.vue'
import { mockApi, mountView, settle } from '../helpers'

const ADMIN = 'aaaaaaaa-0000-4000-8000-000000000001'
const MEMBER = 'aaaaaaaa-0000-4000-8000-000000000002'
const perms = [
  { code: 'crm.delete', description: 'Delete and merge CRM records' },
  { code: 'crm.read', description: 'View CRM' },
  { code: 'users.manage', description: 'Manage users and roles' },
]
const role = (id: string, key: string, name: string, permissions: string[], is_system = true) => ({
  id, key, name, description: '', is_system, permissions, user_count: 1, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z',
})

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

describe('RolesView permission matrix', () => {
  it('saves only the changed role with its full permission list', async () => {
    let member = role(MEMBER, 'member', 'Member', ['crm.read'])
    const api = mockApi({
      'GET /roles': () => ({ items: [role(ADMIN, 'admin', 'Administrator', perms.map((p) => p.code)), member] }),
      'GET /permissions': { items: perms },
      [`PUT /roles/${MEMBER}`]: (c) => {
        member = { ...member, permissions: (c.body as { permissions: string[] }).permissions }
        return member
      },
    })
    const { wrapper } = await mountView(RolesView)
    const adminCell = wrapper.find('[data-testid="perm-admin-crm.delete"]')
    expect((adminCell.element as HTMLInputElement).checked).toBe(true)
    expect(adminCell.attributes('disabled')).toBeDefined()

    const cell = wrapper.find('[data-testid="perm-member-crm.delete"]')
    expect((cell.element as HTMLInputElement).checked).toBe(false)
    await cell.setValue(true)
    expect(wrapper.text()).toContain('1 unsaved change to Member')
    await wrapper.findAll('button').find((b) => b.text() === 'Save permissions')!.trigger('click')
    await settle()

    const puts = api.calls.filter((c) => c.method === 'PUT')
    expect(puts).toHaveLength(1)
    expect(puts[0]!.path).toBe(`/roles/${MEMBER}`)
    expect(puts[0]!.body).toEqual({ key: 'member', name: 'Member', description: '', permissions: ['crm.delete', 'crm.read'] })
    // The list is reloaded and the pending-changes bar disappears.
    expect(api.calls.filter((c) => c.method === 'GET' && c.path === '/roles').length).toBeGreaterThan(1)
    expect(wrapper.text()).not.toContain('unsaved change')
    expect((wrapper.find('[data-testid="perm-member-crm.delete"]').element as HTMLInputElement).checked).toBe(true)
  })

  it('discards pending changes', async () => {
    mockApi({ 'GET /roles': { items: [role(MEMBER, 'member', 'Member', ['crm.read'])] }, 'GET /permissions': { items: perms } })
    const { wrapper } = await mountView(RolesView)
    await wrapper.find('[data-testid="perm-member-crm.read"]').setValue(false)
    await wrapper.findAll('button').find((b) => b.text() === 'Discard')!.trigger('click')
    expect((wrapper.find('[data-testid="perm-member-crm.read"]').element as HTMLInputElement).checked).toBe(true)
    expect(wrapper.text()).not.toContain('unsaved change')
  })
})
