import { afterEach, describe, expect, it, vi } from 'vitest'
import TasksView from '@/views/crm/TasksView.vue'
import TaskDetailView from '@/views/crm/TaskDetailView.vue'
import { mockApi, mountView, settle } from '../helpers'
import { T0, USER_ID, baseRoutes, confirmOk, page, withConfirm } from './fixtures'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

function task(id: string, extra: object = {}) {
  return {
    id, title: `Task ${id}`, description: '', status: 'open', priority: 'normal', task_type: 'call', due_at: null, remind_at: null, assignee_id: USER_ID,
    assignee_name: 'Owner', entity_type: 'company', entity_id: 'c1', completed_at: null, overdue: false, created_at: T0, updated_at: T0, created_by: null, ...extra,
  }
}

describe('TasksView', () => {
  it('filters to my open tasks and completes one with the checkbox', async () => {
    const api = mockApi({ ...baseRoutes, 'GET /tasks': () => page([task('t1')]), 'PUT /tasks/t1': (c) => ({ ...task('t1'), ...(c.body as object) }), 'GET /tasks/reminders': { items: [] } })
    const { wrapper, router } = await mountView(TasksView, { path: '/tasks', pattern: '/tasks' })
    const assignee = wrapper.findAll('select').find((s) => s.find('option[value="me"]').exists())!
    await assignee.setValue('me')
    await settle()
    await wrapper.findAll('input[type="checkbox"]').find((c) => c.element.parentElement?.textContent?.includes('Open only'))!.setValue(true)
    await settle()
    expect(router.currentRoute.value.query).toMatchObject({ assignee_id: 'me', open_only: 'true' })
    const last = api.calls.filter((c) => c.path === '/tasks').at(-1)!
    expect(last.query.get('assignee_id')).toBe('me')
    expect(last.query.get('open_only')).toBe('true')

    await wrapper.find('[data-testid="task-done"]').setValue(true)
    await settle()
    const put = api.calls.find((c) => c.method === 'PUT')!
    expect(put.body).toEqual({
      title: 'Task t1', description: '', status: 'done', priority: 'normal', task_type: 'call', due_at: null, remind_at: null, assignee_id: USER_ID, entity_type: 'company', entity_id: 'c1',
    })
  })

  it('converts the due-before date into an RFC 3339 timestamp', async () => {
    const api = mockApi({ ...baseRoutes, 'GET /tasks': () => page([]) })
    const { wrapper } = await mountView(TasksView, { path: '/tasks', pattern: '/tasks' })
    await wrapper.find('input[type="date"]').setValue('2026-11-30')
    await settle()
    const due = api.calls.filter((c) => c.path === '/tasks').at(-1)!.query.get('due_before')!
    expect(new Date(due).toISOString()).toBe(due)
    expect(new Date(due).getDate()).toBe(30)
    expect(wrapper.text()).toContain('No tasks match these filters')
  })
})

describe('TaskDetailView', () => {
  it('changes status and deletes after confirmation', async () => {
    const api = mockApi({
      ...baseRoutes,
      'GET /tasks/t1': task('t1', { assignee_name: undefined }),
      'GET /companies/c1': { id: 'c1', name: 'Acme GmbH' },
      'PUT /tasks/t1': (c) => ({ ...task('t1'), ...(c.body as object) }),
      'DELETE /tasks/t1': () => undefined,
      'GET /tasks/reminders': { items: [] },
    })
    const { wrapper, router } = await mountView(withConfirm(TaskDetailView), { path: '/tasks/t1', pattern: '/tasks/:id' })
    await settle()
    expect(wrapper.text()).toContain('Acme GmbH')
    expect(wrapper.text()).toContain('Owner')
    await wrapper.findAll('button').find((b) => b.text().includes('Mark done'))!.trigger('click')
    await settle()
    expect((api.calls.find((c) => c.method === 'PUT')!.body as { status: string }).status).toBe('done')
    await wrapper.findAll('button').find((b) => b.text().includes('Delete'))!.trigger('click')
    await settle()
    expect(api.calls.some((c) => c.method === 'DELETE')).toBe(false)
    confirmOk()
    await settle()
    expect(api.calls.some((c) => c.method === 'DELETE' && c.path === '/tasks/t1')).toBe(true)
    expect(router.currentRoute.value.path).toBe('/tasks')
  })
})
