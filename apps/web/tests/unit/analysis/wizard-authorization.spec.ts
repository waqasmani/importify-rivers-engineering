import { afterEach, describe, expect, it, vi } from 'vitest'
import NewAnalysisView from '@/views/analysis/NewAnalysisView.vue'
import { apiError, mockApi, mountView, settle, type Call } from '../helpers'
import { analyzersDoc, PROJECT_ID, project, systemInfo, WS_ID } from './fixtures'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

function routes(extra: Record<string, unknown> = {}, analyzers = analyzersDoc()) {
  let n = 0
  return {
    'GET /analysis/analyzers': analyzers,
    'GET /system/info': systemInfo,
    'GET /workspaces': { items: [{ id: WS_ID, name: 'Default', slug: 'default', description: '', project_count: 0, created_at: '', updated_at: '', created_by: null }] },
    'GET /users/directory': { items: [] },
    'POST /analysis/projects': (c: Call) => ({ ...project(), ...(c.body as object), id: PROJECT_ID }),
    [`POST /analysis/projects/${PROJECT_ID}/sources`]: (c: Call) => ({ source: { id: `src-json-${++n}`, kind: (c.body as { kind: string }).kind, title: 'Pasted text', project_id: PROJECT_ID }, run: null }),
    [`POST /analysis/projects/${PROJECT_ID}/sources/upload`]: (c: Call) => ({ source: { id: `src-up-${++n}`, kind: (c.body as FormData).get('kind'), title: 'openapi.yaml', project_id: PROJECT_ID }, run: null }),
    [`POST /analysis/projects/${PROJECT_ID}/runs`]: (c: Call) => ({ id: 'run', status: 'queued', ...(c.body as object) }),
    ...extra,
  }
}

const button = (w: { findAll: (s: string) => { text: () => string; trigger: (e: string) => Promise<void>; attributes: (a: string) => string | undefined }[] }, text: string) => w.findAll('button').find((b) => b.text().trim().startsWith(text))!

describe('NewAnalysisView', () => {
  it('only offers server-side URL fetching when the operator enabled it', async () => {
    mockApi(routes())
    const { wrapper } = await mountView(NewAnalysisView, { path: '/analysis/new' })
    await wrapper.find('input[maxlength="200"]').setValue('Site review')
    await button(wrapper, 'Next: sources').trigger('click')
    await settle()
    expect(wrapper.find('[data-testid="kind-url"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="kind-text"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('Fetching public URLs from the server is turned off')

    vi.unstubAllGlobals()
    document.body.innerHTML = ''
    mockApi(routes({}, analyzersDoc({ server_fetch: true })))
    const second = await mountView(NewAnalysisView, { path: '/analysis/new' })
    await second.wrapper.find('input[maxlength="200"]').setValue('Site review')
    await button(second.wrapper, 'Next: sources').trigger('click')
    await settle()
    expect(second.wrapper.find('[data-testid="kind-url"]').exists()).toBe(true)
  })

  it('requires authorization for the project and every source before anything is stored', async () => {
    const api = mockApi(routes())
    const { wrapper, router } = await mountView(NewAnalysisView, { path: '/analysis/new' })

    await wrapper.find('input[maxlength="200"]').setValue('Inventory API review')
    await button(wrapper, 'Next: sources').trigger('click')
    await settle()

    // A pasted text source.
    await wrapper.find('[data-testid="kind-text"] input').setValue(true)
    await wrapper.find('textarea').setValue('The system must export orders as CSV.')
    await wrapper.find('[data-testid="add-to-list"]').trigger('click')
    // An uploaded OpenAPI file.
    await wrapper.find('[data-testid="kind-openapi"] input').setValue(true)
    await wrapper.find('input[type="radio"][value="upload"]').setValue(true)
    const file = new File(['openapi: 3.0.0\ninfo: {title: Inv, version: "1"}\npaths: {}\n'], 'openapi.yaml', { type: 'application/yaml' })
    const input = wrapper.find('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [file] })
    await input.trigger('change')
    await wrapper.find('[data-testid="add-to-list"]').trigger('click')
    expect(wrapper.findAll('[data-testid="queued-source"]')).toHaveLength(2)

    await button(wrapper, 'Next: authorization').trigger('click')
    await settle()
    expect(wrapper.find('[data-testid="policy"]').text()).toContain('own or are authorized to examine')
    const submit = () => wrapper.find('[data-testid="submit-authorization"]')
    expect(submit().attributes('disabled')).toBeDefined()

    // Basis that needs a note: still blocked until the note, every source and the final box are confirmed.
    await wrapper.find('input[value="client_authorized"]').setValue(true)
    await wrapper.find('[data-testid="authorization-confirmed"]').setValue(true)
    const sourceBoxes = wrapper.findAll('input[data-testid^="confirm-"]')
    expect(sourceBoxes).toHaveLength(2)
    await sourceBoxes[0]!.setValue(true)
    expect(submit().attributes('disabled')).toBeDefined()
    await sourceBoxes[1]!.setValue(true)
    expect(submit().attributes('disabled')).toBeDefined()
    await wrapper.find('textarea[maxlength="2000"]').setValue('Authorized by Jane Roe, CTO of Example Ltd (contract 42).')
    expect(submit().attributes('disabled')).toBeUndefined()
    expect(api.calls.filter((c) => c.method === 'POST')).toHaveLength(0)

    await submit().trigger('click')
    await settle(6)
    const created = api.calls.find((c) => c.method === 'POST' && c.path === '/analysis/projects')!
    expect(created.body).toMatchObject({
      name: 'Inventory API review', workspace_id: WS_ID, authorization_basis: 'client_authorized', authorization_confirmed: true,
      authorization_note: 'Authorized by Jane Roe, CTO of Example Ltd (contract 42).',
    })
    const json = api.calls.find((c) => c.path === `/analysis/projects/${PROJECT_ID}/sources`)!
    expect(json.body).toMatchObject({ kind: 'text', content: 'The system must export orders as CSV.', auto_analyze: false, fetch: false })
    const upload = api.calls.find((c) => c.path.endsWith('/sources/upload'))!.body as FormData
    expect(upload.get('kind')).toBe('openapi')
    expect(upload.get('auto_analyze')).toBe('false')
    expect((upload.get('file') as File).name).toBe('openapi.yaml')

    // Step 4: recommended analyzers are preselected, and runs start only on request.
    expect(wrapper.find('[data-testid="start-runs"]').text()).toContain('Start 2 runs')
    await wrapper.find('[data-testid="start-runs"]').trigger('click')
    await settle(6)
    const runs = api.calls.filter((c) => c.path.endsWith('/runs')).map((c) => c.body)
    expect(runs).toEqual([{ source_id: 'src-json-1', analyzer: 'document' }, { source_id: 'src-up-2', analyzer: 'openapi' }])
    expect(router.currentRoute.value.fullPath).toBe(`/analysis/${PROJECT_ID}?tab=runs`)
  })

  it('returns to the project step with server validation errors next to the field', async () => {
    mockApi(routes({ 'POST /analysis/projects': () => apiError(422, 'validation_failed', 'Validation failed', { name: 'is already used' }) }))
    const { wrapper } = await mountView(NewAnalysisView, { path: '/analysis/new' })
    await wrapper.find('input[maxlength="200"]').setValue('Dup')
    await button(wrapper, 'Next: sources').trigger('click')
    await settle()
    await wrapper.find('[data-testid="kind-text"] input').setValue(true)
    await wrapper.find('textarea').setValue('Some text')
    await button(wrapper, 'Next: authorization').trigger('click')
    await settle()
    await wrapper.find('input[value="owned"]').setValue(true)
    await wrapper.find('input[data-testid^="confirm-"]').setValue(true)
    await wrapper.find('[data-testid="authorization-confirmed"]').setValue(true)
    await wrapper.find('[data-testid="submit-authorization"]').trigger('click')
    await settle(5)
    expect(wrapper.find('#step-project').exists()).toBe(true)
    expect(wrapper.text()).toContain('is already used')
  })
})
