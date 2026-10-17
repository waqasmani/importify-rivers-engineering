import { afterEach, describe, expect, it, vi } from 'vitest'
import SourceDetailView from '@/views/analysis/SourceDetailView.vue'
import { mockApi, mountView } from '../helpers'
import { finding, page, PROJECT_ID, project, SOURCE_ID } from './fixtures'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

const source = (kind: string, over: object = {}) => ({
  id: SOURCE_ID, project_id: PROJECT_ID, kind, title: `${kind} source`, url: null, captured_page_id: null, attachment_id: null, content_text: '', content_length: 0,
  content_sha256: 'abc', mime_type: '', size_bytes: 0, metadata: {}, status: 'ready', error: '', created_by: null, created_at: '2026-10-01T10:00:00Z', ...over,
})

function routes(src: object, findings: object[]) {
  return {
    [`GET /analysis/sources/${SOURCE_ID}`]: src,
    [`GET /analysis/projects/${PROJECT_ID}`]: { project: project(), workspace: { id: 'w', name: 'Default' }, finding_counts: [], tags: [] },
    [`GET /analysis/projects/${PROJECT_ID}/findings`]: page(findings),
    [`GET /analysis/projects/${PROJECT_ID}/runs`]: { items: [] },
    'GET /users/directory': { items: [] },
    'GET /ai/status': { enabled: true, configured: false, default_provider: null, message: 'No AI provider is configured. Every other feature works without AI; an administrator can add Ollama or another provider under AI settings.' },
    'GET /ai/tools': { items: [{ key: 'summarize', label: 'Summarize', description: '', entity_types: ['analysis_source'], apply: 'summary' }], suggested_models: {} },
    'GET /ai/jobs': { items: [], total: 0, limit: 10, offset: 0 },
  }
}
const mount = () => mountView(SourceDetailView, { path: `/analysis/sources/${SOURCE_ID}`, pattern: '/analysis/sources/:id' })

describe('SourceDetailView', () => {
  it('shows OpenAPI sources as an endpoint table with parameters, auth and responses', async () => {
    mockApi(routes(source('openapi', { content_text: 'openapi: 3.0.0' }), [
      finding('e1', 'verified', { data: { method: 'GET', path: '/items/{id}', summary: 'Get an item', parameters: ['id (path, required)'], responses: ['200 Item', '404'], secured: true, tags: ['items'] } }),
      finding('e2', 'verified', { data: { method: 'POST', path: '/items', summary: 'Create an item', parameters: [], request_body: 'application/json: Item', responses: ['201 Item'], secured: false } }),
      finding('s1', 'verified', { category: 'schema', title: 'Schema: Item', data: { name: 'Item', fields: [{ name: 'sku', type: 'string', required: true }] } }),
    ]))
    const { wrapper } = await mount()
    const rows = wrapper.findAll('[data-testid="api-endpoint"]')
    expect(rows).toHaveLength(2)
    // Sorted by path, then method.
    rows.reverse()
    expect(rows[0]!.text()).toContain('GET')
    expect(rows[0]!.text()).toContain('/items/{id}')
    expect(rows[0]!.text()).toContain('id (path, required)')
    expect(rows[0]!.text()).toContain('Required')
    expect(rows[0]!.text()).toContain('200 Item')
    expect(rows[1]!.text()).toContain('application/json: Item')
    expect(rows[1]!.text()).toContain('None declared')
    expect(wrapper.text()).toContain('Schemas (1)')
    await wrapper.find('#api-method').setValue('POST')
    expect(wrapper.findAll('[data-testid="api-endpoint"]')).toHaveLength(1)
    expect(wrapper.find('[data-testid="source-authorization"]').text()).toContain('We own it')
    expect(wrapper.text()).toContain('No AI provider is configured')
  })

  it('shows screenshots with the visible-content-only disclaimer', async () => {
    mockApi(routes(source('screenshot', { attachment_id: 'att1', mime_type: 'image/png' }), [finding('i1', 'verified', { category: 'image', data: { width: 1280, height: 800, format: 'png' } })]))
    const { wrapper } = await mount()
    expect(wrapper.find('[data-testid="visible-only-notice"]').text()).toContain('Visible behaviour only')
    expect(wrapper.find('img').attributes('src')).toBe('/api/v1/attachments/att1/download?inline=1')
    expect(wrapper.text()).toContain('1280 × 800 px')
  })

  it('builds a codebase tree from the analyzer findings and states code is not executed', async () => {
    mockApi(routes(source('repository_archive', { attachment_id: 'att2', size_bytes: 2048 }), [
      finding('l1', 'verified', { category: 'language', data: { files: { Go: 3, Markdown: 1 }, lines: { Go: 120, Markdown: 10 } } }),
      finding('d1', 'verified', { category: 'structure', data: { directories: { cmd: 1, internal: 2, '(root)': 1 } } }),
      finding('r1', 'verified', { category: 'route', data: { method: 'GET', path: '/health', file: 'internal/http/server.go', line: 12, framework: 'net/http' } }),
      finding('m1', 'inferred', { category: 'entry_point', title: 'Entry point: cmd/app/main.go', data: { file: 'cmd/app/main.go' } }),
      finding('p1', 'verified', { category: 'dependency', data: { manifest: 'go.mod', dependencies: [{ manifest: 'go.mod', name: 'github.com/go-chi/chi/v5', version: 'v5.0.0' }] } }),
    ]))
    const { wrapper } = await mount()
    expect(wrapper.find('[data-testid="codebase-viewer"]').text()).toContain('never executed')
    const tree = wrapper.find('[data-testid="file-tree"]').text()
    expect(tree).toContain('internal/')
    expect(tree).toContain('server.go')
    expect(tree).toContain('main.go')
    expect(tree).toContain('go.mod')
    expect(wrapper.text()).toContain('github.com/go-chi/chi/v5')
    expect(wrapper.text()).toContain('/health')
    expect(wrapper.text()).toContain('Go')
  })

  it('searches extracted document text', async () => {
    mockApi(routes(source('document', { content_text: 'Intro\nThe system must export CSV.\nThe system should log events.\nEnd', content_length: 64 }), []))
    const { wrapper } = await mount()
    await wrapper.find('#text-search').setValue('system')
    await vi.waitFor(() => expect(wrapper.find('[data-testid="text-hits"]').exists()).toBe(true), { timeout: 1000 })
    expect(wrapper.find('[data-testid="text-hits"]').text()).toContain('2 matches on 2 lines')
    expect(wrapper.findAll('mark')).toHaveLength(2)
  })
})
