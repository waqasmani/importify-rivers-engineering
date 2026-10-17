import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import ReportView from '@/views/analysis/ReportView.vue'
import ConfirmHost from '@/components/ui/ConfirmHost.vue'
import { mockApi, mountView, settle } from '../helpers'
import { finding, PROJECT_ID, project } from './fixtures'

const REPORT_ID = '44444444-4444-4444-8444-444444444444'

const report = (over: object = {}) => ({
  id: REPORT_ID, project_id: PROJECT_ID, kind: 'api_inventory', title: 'API inventory', content_markdown: '# API inventory\n\nGET /items [F-f1aaaa]',
  origin: 'rule', is_ai_generated: false, ai_job_id: null, provider_name: '', model: '', finding_ids: ['f1aaaaaa', 'f2bbbbbb'], source_ids: [], created_by: null,
  created_at: '2026-10-02T10:00:00Z', ...over,
})

const Host = defineComponent({ props: { id: { type: String, required: true } }, setup: (p) => () => h('div', [h(ReportView, { id: p.id }), h(ConfirmHost)]) })

function routes(rep: object) {
  return {
    [`GET /analysis/reports/${REPORT_ID}`]: { report: rep, findings: [finding('f1aaaaaa', 'verified', { title: 'GET /items' }), finding('f2bbbbbb', 'inferred', { title: 'Cursor pagination' })] },
    [`GET /analysis/projects/${PROJECT_ID}`]: { project: project(), workspace: { id: 'w', name: 'Default' }, finding_counts: [], tags: [] },
    [`GET /analysis/reports/${REPORT_ID}/export`]: (c: { query: URLSearchParams }) =>
      new Response(c.query.get('format') === 'json' ? '{"report":{}}' : '# API inventory', {
        status: 200,
        headers: { 'Content-Type': c.query.get('format') === 'json' ? 'application/json' : 'text/markdown', 'Content-Disposition': `attachment; filename="api-inventory.${c.query.get('format')}"` },
      }),
    [`DELETE /analysis/reports/${REPORT_ID}`]: () => undefined,
  }
}

let clicked: string[] = []
beforeEach(() => {
  clicked = []
  vi.stubGlobal('URL', Object.assign(URL, { createObjectURL: () => 'blob:test', revokeObjectURL: () => {} }))
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) {
    clicked.push(this.download)
  })
})
afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

describe('ReportView', () => {
  it('labels AI-generated reports prominently with provider and model', async () => {
    mockApi(routes(report({ is_ai_generated: true, origin: 'ai', provider_name: 'Local Ollama', model: 'llama3.1', ai_job_id: 'job1' })))
    const { wrapper } = await mountView(ReportView, { path: `/analysis/reports/${REPORT_ID}`, pattern: '/analysis/reports/:id' })
    const banner = wrapper.find('[data-testid="ai-banner"]')
    expect(banner.exists()).toBe(true)
    expect(banner.text()).toContain('AI-generated — verify against the cited evidence')
    expect(banner.text()).toContain('Local Ollama / llama3.1')
    expect(wrapper.find('[data-testid="deterministic-banner"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('llama3.1')
  })

  it('marks deterministic reports, indexes cited evidence and exports Markdown and JSON', async () => {
    const api = mockApi(routes(report()))
    const { wrapper } = await mountView(ReportView, { path: `/analysis/reports/${REPORT_ID}`, pattern: '/analysis/reports/:id' })
    expect(wrapper.find('[data-testid="ai-banner"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="deterministic-banner"]').text()).toContain('Deterministic report generated from findings')
    expect(wrapper.find('.prose-rea').text()).toContain('GET /items [F-f1aaaa]')

    const index = wrapper.find('[data-testid="evidence-index"]')
    expect(index.text()).toContain('[F-f1aaaa]')
    expect(index.find('[data-testid="cited-f2bbbbbb"] [data-testid="classification"]').text()).toBe('Inferred')
    expect(index.find('[data-testid="cited-f1aaaaaa"] a').attributes('href')).toBe('/analysis/findings/f1aaaaaa')

    await wrapper.findAll('button').find((b) => b.text().includes('Markdown'))!.trigger('click')
    await settle()
    await wrapper.findAll('button').find((b) => b.text().includes('JSON'))!.trigger('click')
    await settle()
    const exports = api.calls.filter((c) => c.path.endsWith('/export')).map((c) => c.query.get('format'))
    expect(exports).toEqual(['md', 'json'])
    expect(clicked).toEqual(['api-inventory.md', 'api-inventory.json'])
  })

  it('deletes after confirmation and returns to the project reports', async () => {
    const api = mockApi(routes(report()))
    const { wrapper, router } = await mountView(Host, { path: `/analysis/reports/${REPORT_ID}`, pattern: '/analysis/reports/:id' })
    await wrapper.findAll('button').find((b) => b.text().includes('Delete'))!.trigger('click')
    await settle()
    expect(document.body.textContent).toContain('Delete report')
    ;(document.querySelector('[data-testid="confirm-ok"]') as HTMLButtonElement).click()
    await settle(5)
    expect(api.calls.some((c) => c.method === 'DELETE')).toBe(true)
    expect(router.currentRoute.value.fullPath).toBe(`/analysis/${PROJECT_ID}?tab=reports`)
  })

  it('hides delete without write permission', async () => {
    mockApi(routes(report()))
    const { wrapper } = await mountView(ReportView, { path: `/analysis/reports/${REPORT_ID}`, pattern: '/analysis/reports/:id', permissions: ['analysis.read'] })
    expect(wrapper.findAll('button').some((b) => b.text().includes('Delete'))).toBe(false)
    expect(wrapper.findAll('button').some((b) => b.text().includes('Markdown'))).toBe(true)
  })
})
