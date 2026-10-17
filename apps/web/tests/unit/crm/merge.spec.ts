import { afterEach, describe, expect, it, vi } from 'vitest'
import CompanyDetailView from '@/views/crm/CompanyDetailView.vue'
import DuplicatesView from '@/views/crm/DuplicatesView.vue'
import { groupDuplicates } from '@/components/crm/duplicates'
import { mockApi, mountView, settle } from '../helpers'
import { baseRoutes, company, confirmOk, page, withConfirm } from './fixtures'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

const ID = '11111111-1111-4111-8111-111111111111'
const DUP = '22222222-2222-4222-8222-222222222222'

function detailRoutes(extra: object = {}) {
  return {
    ...baseRoutes,
    [`GET /companies/${ID}`]: company(ID, 'Acme GmbH', { domain: 'acme.test' }),
    'GET /contacts': page([]),
    'GET /leads': page([]),
    'GET /opportunities': page([]),
    'GET /suppliers': page([]),
    'GET /notes': { items: [] },
    ...extra,
  }
}

describe('CompanyDetailView merge', () => {
  it('finds duplicates, confirms what is merged and posts the source ids', async () => {
    const api = mockApi(
      detailRoutes({
        'GET /companies/duplicates': { items: [{ id: DUP, label: 'ACME Gmbh.', score: 1, reasons: ['same domain acme.test'], record: company(DUP, 'ACME Gmbh.', { domain: 'acme.test', country: 'DE' }) }] },
        [`POST /companies/${ID}/merge`]: company(ID, 'Acme GmbH'),
      }),
    )
    const { wrapper } = await mountView(withConfirm(CompanyDetailView), { path: `/companies/${ID}`, pattern: '/companies/:id' })
    expect(wrapper.find('h1').text()).toBe('Acme GmbH')
    await wrapper.findAll('button').find((b) => b.text().includes('Find duplicates'))!.trigger('click')
    await settle()
    const dupCall = api.calls.find((c) => c.path === '/companies/duplicates')!
    expect(Object.fromEntries(dupCall.query)).toEqual({ exclude_id: ID, name: 'Acme GmbH', domain: 'acme.test' })
    const list = wrapper.find('[data-testid="duplicate-candidates"]')
    expect(list.text()).toContain('ACME Gmbh.')
    expect(list.text()).toContain('100% match')
    expect(list.text()).toContain('same domain acme.test')

    const mergeBtn = wrapper.find('[data-testid="merge-selected"]')
    expect(mergeBtn.attributes('disabled')).toBeDefined()
    await list.find('input[type="checkbox"]').setValue(true)
    await mergeBtn.trigger('click')
    await settle()
    expect(document.body.textContent).toContain('Merge 1 company into “Acme GmbH”?')
    expect(document.body.textContent).toContain('ACME Gmbh. (100% match)')
    expect(api.calls.some((c) => c.method === 'POST')).toBe(false)
    confirmOk()
    await settle()
    expect(api.calls.find((c) => c.method === 'POST')?.body).toEqual({ source_ids: [DUP] })
    // The record is reloaded after the merge.
    expect(api.calls.filter((c) => c.method === 'GET' && c.path === `/companies/${ID}`).length).toBeGreaterThan(1)
  })

  it('hides merge and delete without crm.delete', async () => {
    mockApi(detailRoutes())
    const { wrapper } = await mountView(CompanyDetailView, { path: `/companies/${ID}`, pattern: '/companies/:id', permissions: ['crm.read', 'crm.write'] })
    const labels = wrapper.findAll('button').map((b) => b.text())
    expect(labels.some((t) => t.includes('Find duplicates'))).toBe(false)
    expect(labels.some((t) => t.includes('Delete'))).toBe(false)
    expect(labels.some((t) => t.includes('Edit'))).toBe(true)
  })

  it("loads the company's suppliers with the company_id filter and links to the filtered list", async () => {
    const supplier = (n: number) => ({ id: `5555555${n}-5555-4555-8555-555555555555`, name: `Supplier ${n}`, supplier_type: 'manufacturer', country: 'DE', company_id: ID })
    const api = mockApi(detailRoutes({ 'GET /suppliers': { items: [supplier(1), supplier(2)], total: 12, limit: 10, offset: 0 } }))
    const { wrapper } = await mountView(CompanyDetailView, { path: `/companies/${ID}`, pattern: '/companies/:id' })
    const calls = api.calls.filter((c) => c.path === '/suppliers')
    expect(calls).toHaveLength(1)
    expect(Object.fromEntries(calls[0]!.query)).toEqual({ company_id: ID, limit: '10' })
    const section = wrapper.get('section[aria-label="Suppliers"]')
    expect(section.text()).toContain('(12)')
    expect(section.text()).toContain('Supplier 2')
    expect(section.text()).not.toContain('records were checked')
    expect(section.get('a[href^="/suppliers?"]').attributes('href')).toBe(`/suppliers?company_id=${ID}`)
  })
})

describe('DuplicatesView', () => {
  it('groups mutual matches once and merges into the chosen primary', async () => {
    const a = company('a', 'Acme', { created_at: '2026-01-01T00:00:00Z', domain: 'acme.test' })
    const b = company('b', 'Acme Inc', { created_at: '2026-02-01T00:00:00Z', domain: 'acme.test' })
    const c = company('c', 'Other', { created_at: '2026-03-01T00:00:00Z' })
    const api = mockApi({
      ...baseRoutes,
      'GET /companies': page([c, b, a]),
      'GET /companies/duplicates': (call) => {
        const ex = call.query.get('exclude_id')
        if (ex === 'a') return { items: [{ id: 'b', label: 'Acme Inc', score: 1, reasons: ['same domain acme.test'], record: b }] }
        if (ex === 'b') return { items: [{ id: 'a', label: 'Acme', score: 1, reasons: ['same domain acme.test'], record: a }] }
        return { items: [] }
      },
      'POST /companies/b/merge': company('b', 'Acme Inc'),
    })
    const { wrapper } = await mountView(withConfirm(DuplicatesView), { path: '/duplicates', pattern: '/duplicates' })
    await settle(6)
    const groups = wrapper.findAll('[data-testid="duplicate-groups"] > li')
    expect(groups).toHaveLength(1)
    expect(groups[0]!.text()).toContain('2 similar companies')
    expect(groups[0]!.text()).toContain('same domain acme.test')
    // Oldest record is the default primary; switch to the newer one.
    const radios = groups[0]!.findAll('input[type="radio"]')
    await radios[1]!.setValue(true)
    await groups[0]!.findAll('button').find((x) => x.text() === 'Merge into primary')!.trigger('click')
    await settle()
    expect(document.body.textContent).toContain('Merge 1 into “Acme Inc”?')
    confirmOk()
    await settle()
    expect(api.calls.find((x) => x.method === 'POST')).toMatchObject({ path: '/companies/b/merge', body: { source_ids: ['a'] } })
  })

  it('joins chains of matches into one group', () => {
    const rec = (id: string) => ({ id, name: id, created_at: `2026-0${id.charCodeAt(0) - 96}-01T00:00:00Z` })
    const groups = groupDuplicates('company', [
      { record: rec('a'), candidates: [{ id: 'b', label: 'b', score: 0.6, reasons: ['similar name'], record: rec('b') }] },
      { record: rec('c'), candidates: [{ id: 'b', label: 'b', score: 0.9, reasons: null as unknown as string[], record: rec('b') }] },
    ])
    expect(groups).toHaveLength(1)
    expect(groups[0]!.members.map((m) => m.id)).toEqual(['a', 'b', 'c'])
    expect(groups[0]!.score).toBe(0.9)
    expect(groups[0]!.evidence[0]!.reasons).toEqual([])
  })
})
