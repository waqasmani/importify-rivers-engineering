import { afterEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, ref } from 'vue'
import RunsPanel from '@/components/analysis/RunsPanel.vue'
import ConfirmHost from '@/components/ui/ConfirmHost.vue'
import { useProjectRuns } from '@/components/analysis/useProjectRuns'
import { mockApi, mountView, settle } from '../helpers'
import { PROJECT_ID, SOURCE_ID } from './fixtures'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

const run = (id: string, status: string, over: object = {}) => ({
  id, project_id: PROJECT_ID, source_id: SOURCE_ID, source_title: 'openapi.yaml', analyzer: 'openapi', status, attempts: 1, options: {}, stats: {}, error: '',
  ai_job_id: null, created_by: null, queued_at: '2026-10-01T10:00:00Z', started_at: null, finished_at: null, locked_by: '', locked_at: null, ...over,
})

function host(onFinished: (runs: unknown[]) => void) {
  return defineComponent({
    setup() {
      const runs = useProjectRuns(ref(PROJECT_ID), { pollMs: 60, onFinished })
      return () =>
        h('div', [
          h(RunsPanel, {
            projectId: PROJECT_ID, runs: runs.items.value, loading: runs.loading.value, error: runs.error.value,
            sources: [{ id: SOURCE_ID, title: 'openapi.yaml', kind: 'openapi' }], analyzers: ['openapi', 'document'], canWrite: true, onRefresh: () => runs.refetch(),
          }),
          h(ConfirmHost),
        ])
    },
  })
}

describe('analysis runs', () => {
  it('polls while a run is active, then reports finished runs and stops polling', async () => {
    let calls = 0
    mockApi({
      [`GET /analysis/projects/${PROJECT_ID}/runs`]: () => {
        calls++
        if (calls < 3) return { items: [run('r1', calls === 1 ? 'queued' : 'running', { started_at: '2026-10-01T10:00:01Z' }), run('r0', 'failed', { error: 'the archive was rejected: too many files', analyzer: 'codebase' })] }
        return { items: [run('r1', 'succeeded', { stats: { operations: 12, schemas: 4, findings: 18, duration_ms: 420 }, finished_at: '2026-10-01T10:00:02Z' }), run('r0', 'failed', { error: 'the archive was rejected: too many files', analyzer: 'codebase' })] }
      },
    })
    const finished = vi.fn()
    const { wrapper } = await mountView(host(finished))
    // The first poll may already have happened while mounting; either active state is fine here.
    expect(['queued', 'running']).toContain(wrapper.find('[data-testid="run-status-r1"]').text())
    expect(wrapper.find('[data-testid="runs-active"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="run-r0"]').text()).toContain('the archive was rejected: too many files')

    await vi.waitFor(() => expect(wrapper.find('[data-testid="run-status-r1"]').text()).toBe('succeeded'), { timeout: 4000, interval: 20 })
    await settle()
    expect(finished).toHaveBeenCalledTimes(1)
    expect((finished.mock.calls[0]![0] as { id: string }[]).map((r) => r.id)).toEqual(['r1'])
    const r1 = wrapper.find('[data-testid="run-r1"]').text()
    expect(r1).toContain('operations:')
    expect(r1).toContain('12')
    expect(r1).toContain('420 ms')
    expect(wrapper.find('[data-testid="runs-active"]').exists()).toBe(false)

    const after = calls
    await new Promise((r) => setTimeout(r, 250))
    expect(calls).toBe(after)
  })

  it('cancels a queued run after confirmation and starts a new run with the chosen analyzer', async () => {
    let cancelled = false
    const api = mockApi({
      [`GET /analysis/projects/${PROJECT_ID}/runs`]: () => ({ items: [run('r2', cancelled ? 'cancelled' : 'queued')] }),
      'POST /analysis/runs/r2/cancel': () => {
        cancelled = true
        return undefined
      },
      [`POST /analysis/projects/${PROJECT_ID}/runs`]: (c) => ({ ...run('r3', 'queued'), ...(c.body as object) }),
    })
    const { wrapper } = await mountView(host(() => {}))
    await wrapper.find('[data-testid="run-r2"]').findAll('button').find((b) => b.text().includes('Cancel'))!.trigger('click')
    await settle()
    expect(document.body.textContent).toContain('Cancel the queued')
    ;(document.querySelector('[data-testid="confirm-ok"]') as HTMLButtonElement).click()
    await settle(5)
    expect(api.calls.some((c) => c.path === '/analysis/runs/r2/cancel' && c.method === 'POST')).toBe(true)
    await vi.waitFor(() => expect(wrapper.find('[data-testid="run-status-r2"]').text()).toBe('cancelled'))

    const selects = wrapper.findAll('form select')
    expect((selects[1]!.element as HTMLSelectElement).value).toBe('openapi')
    await selects[1]!.setValue('document')
    await wrapper.find('form').trigger('submit')
    await settle()
    expect(api.calls.find((c) => c.method === 'POST' && c.path.endsWith('/runs'))!.body).toEqual({ source_id: SOURCE_ID, analyzer: 'document' })
  })
})
