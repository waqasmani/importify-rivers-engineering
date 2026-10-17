import { afterEach, describe, expect, it, vi } from 'vitest'
import FindingsBrowser from '@/components/analysis/FindingsBrowser.vue'
import FindingSummary from '@/components/analysis/FindingSummary.vue'
import { mockApi, mountView, settle } from '../helpers'
import { finding, page, PROJECT_ID, SOURCE_ID } from './fixtures'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

const path = `/analysis/projects/${PROJECT_ID}/findings`
const rows = [
  finding('f1', 'verified', { evidence: 'GET /items returns 200', evidence_locator: 'paths./items.get' }),
  finding('f2', 'inferred', { origin: 'ai', title: 'Pagination uses cursors' }),
  finding('f3', 'unknown', { evidence: '', detail: 'Implementation is not described', category: 'backend' }),
]

describe('FindingsBrowser', () => {
  it('renders each classification distinctly with confidence, evidence and locator', async () => {
    mockApi({ [`GET ${path}`]: page(rows) })
    const { wrapper } = await mountView(FindingsBrowser, { path: '/p', props: { projectId: PROJECT_ID, sources: [{ id: SOURCE_ID, title: 'openapi.yaml' }], categories: ['backend', 'endpoint'] } })
    const row = (id: string) => wrapper.find(`[data-testid="row-${id}"]`)
    const badge = (id: string) => row(id).find('[data-testid="classification"]')
    expect(badge('f1').attributes('data-classification')).toBe('verified')
    expect(badge('f1').text()).toBe('Verified')
    expect(badge('f2').text()).toBe('Inferred')
    expect(badge('f3').text()).toBe('Unknown')
    // Visual styles differ, not only colour: solid, dashed and dotted borders.
    expect(badge('f1').classes()).toContain('border-solid')
    expect(badge('f2').classes()).toContain('border-dashed')
    expect(badge('f3').classes()).toContain('border-dotted')
    expect(row('f1').find('[role="meter"]').attributes('aria-valuenow')).toBe('95')
    expect(row('f1').text()).toContain('GET /items returns 200')
    expect(row('f1').text()).toContain('paths./items.get')
    expect(row('f2').text()).toContain('AI-generated')
    expect(row('f3').text()).toContain('Implementation is not described')
    expect(row('f1').find('a').attributes('href')).toBe('/analysis/findings/f1')
  })

  it('sends filters to the API and keeps them in the URL', async () => {
    const api = mockApi({ [`GET ${path}`]: page(rows) })
    const { wrapper, router } = await mountView(FindingsBrowser, { path: '/p', props: { projectId: PROJECT_ID, sources: [{ id: SOURCE_ID, title: 'openapi.yaml' }], categories: ['backend', 'endpoint'] } })
    await wrapper.find('#findings-classification').setValue('inferred')
    await settle()
    expect(router.currentRoute.value.query.classification).toBe('inferred')
    await wrapper.find('#findings-category').setValue('endpoint')
    await settle()
    await wrapper.find('#findings-source').setValue(SOURCE_ID)
    await settle()
    await wrapper.find('#findings-origin').setValue('manual')
    await settle()
    await wrapper.find('#findings-q').setValue('pagination')
    await wrapper.find('form[role="search"]').trigger('submit')
    await settle()
    const last = api.calls.filter((c) => c.path === path).at(-1)!
    expect(Object.fromEntries(last.query)).toMatchObject({ classification: 'inferred', category: 'endpoint', source_id: SOURCE_ID, origin: 'manual', q: 'pagination' })
    expect(router.currentRoute.value.query).toMatchObject({ classification: 'inferred', category: 'endpoint', source_id: SOURCE_ID, origin: 'manual', q: 'pagination' })

    await wrapper.findAll('button').find((b) => b.text() === 'Clear filters')!.trigger('click')
    await settle()
    expect(router.currentRoute.value.query).toEqual({})
    expect([...api.calls.at(-1)!.query.keys()]).toEqual(['limit'])
  })

  it('pins the source on a source page and hides add for read-only users', async () => {
    const api = mockApi({ [`GET ${path}`]: page([]) })
    const { wrapper } = await mountView(FindingsBrowser, { path: '/p', props: { projectId: PROJECT_ID, sources: [], sourceId: SOURCE_ID }, permissions: ['analysis.read'] })
    expect(api.calls[0]!.query.get('source_id')).toBe(SOURCE_ID)
    expect(wrapper.find('#findings-source').exists()).toBe(false)
    expect(wrapper.findAll('button').some((b) => b.text().includes('Add finding'))).toBe(false)
    expect(wrapper.text()).toContain('No findings yet')
  })

  it('validates evidence rules before creating a manual finding', async () => {
    const api = mockApi({ [`GET ${path}`]: page([]), [`POST ${path}`]: (c) => ({ ...finding('new', 'inferred'), ...(c.body as object) }), [`GET ${path}/summary`]: { items: [] } })
    const { wrapper } = await mountView(FindingsBrowser, { path: '/p', props: { projectId: PROJECT_ID, sources: [{ id: SOURCE_ID, title: 'openapi.yaml' }], canWrite: true } })
    await wrapper.findAll('button').find((b) => b.text().includes('Add finding'))!.trigger('click')
    await settle()
    const form = wrapper.find('#finding-form')
    await form.find('input[maxlength="500"]').setValue('Orders are exported nightly')
    await form.find('input[maxlength="41"]').setValue('Business Rule')
    await form.find('input[value="inferred"]').setValue(true)
    await form.trigger('submit')
    await settle()
    expect(wrapper.text()).toContain('Inferred findings need their assumptions.')
    expect(api.calls.some((c) => c.method === 'POST')).toBe(false)
    const areas = form.findAll('textarea')
    await areas[1]!.setValue('The export job seen in the UI runs on a schedule.')
    await form.trigger('submit')
    await settle()
    const post = api.calls.find((c) => c.method === 'POST')!
    expect(post.body).toMatchObject({ title: 'Orders are exported nightly', category: 'business_rule', classification: 'inferred', confidence: '0.900', source_id: null })
  })
})

describe('FindingSummary', () => {
  it('counts findings by category and classification and opens filtered lists', async () => {
    const { wrapper } = await mountView(FindingSummary, {
      props: { rows: [{ category: 'endpoint', classification: 'verified', finding_count: 5 }, { category: 'endpoint', classification: 'inferred', finding_count: 2 }, { category: 'backend', classification: 'unknown', finding_count: 1 }] },
    })
    const text = wrapper.text()
    expect(text).toContain('endpoint')
    expect(text).toContain('backend')
    await wrapper.find('button[aria-label="2 inferred endpoint findings"]').trigger('click')
    expect(wrapper.findComponent(FindingSummary).emitted('open')?.[0]).toEqual([{ category: 'endpoint', classification: 'inferred' }])
  })
})
