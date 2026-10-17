import { afterEach, describe, expect, it, vi } from 'vitest'
import PipelineBoardView from '@/views/crm/PipelineBoardView.vue'
import { mockApi, mountView, settle } from '../helpers'
import { T0, baseRoutes, page } from './fixtures'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

const stages = [
  { id: 's-open', name: 'Proposal', category: 'open', color: '#3366ff', position: 0, pipeline_id: 'p1', probability: 40, created_at: T0 },
  { id: 's-neg', name: 'Negotiation', category: 'open', color: '#ffaa00', position: 1, pipeline_id: 'p1', probability: 70, created_at: T0 },
  { id: 's-lost', name: 'Lost', category: 'lost', color: '#ff0000', position: 2, pipeline_id: 'p1', probability: 0, created_at: T0 },
]
const pipelines = { items: [{ id: 'p1', name: 'Sales', entity_type: 'opportunity', is_default: true, position: 0, created_at: T0, updated_at: T0, stages }] }

function opp(id: string, name: string, stageId: string, extra: object = {}) {
  return {
    id, name, company_id: null, supplier_id: null, contact_id: null, lead_id: null, pipeline_stage_id: stageId, amount: '1000.00', currency: 'EUR', expected_close_date: null,
    probability: 40, status: 'open', closed_at: null, outcome_reason: '', description: 'desc', owner_id: null, custom_fields: { ref: 'X1' }, tags: [], created_at: T0, updated_at: T0, created_by: null, ...extra,
  }
}

function routes(extra: object = {}) {
  const deal = opp('o1', 'Container order', 's-open')
  return {
    ...baseRoutes,
    'GET /pipelines': pipelines,
    'GET /pipelines/p1/summary': {
      pipeline: pipelines.items[0],
      stages: stages.map((s, i) => ({ stage_id: s.id, stage_name: s.name, category: s.category, color: s.color, position: i, deal_count: s.id === 's-open' ? 1 : 0, total_amount: s.id === 's-open' ? '1000' : '0' })),
    },
    'GET /opportunities': (c: { query: URLSearchParams }) => (c.query.get('stage_id') === 's-open' ? page([deal]) : page([])),
    'PATCH /opportunities/o1/stage': (c: { body: unknown }) => ({ ...deal, pipeline_stage_id: (c.body as { stage_id: string }).stage_id, probability: 0, status: 'lost' }),
    'PUT /opportunities/o1': (c: { body: unknown }) => ({ ...deal, ...(c.body as object) }),
    ...extra,
  }
}

describe('PipelineBoardView', () => {
  it('shows one column per stage with totals and loads cards per stage', async () => {
    const api = mockApi(routes())
    const { wrapper } = await mountView(PipelineBoardView, { path: '/pipelines', pattern: '/pipelines' })
    await settle()
    const cols = wrapper.findAll('[data-testid^="column-"]')
    expect(cols.map((c) => c.find('h2').text())).toEqual(['Proposal', 'Negotiation', 'Lost'])
    expect(cols[0]!.text()).toContain('Container order')
    expect(cols[0]!.text()).toContain('in 1 deals')
    const stageQueries = api.calls.filter((c) => c.path === '/opportunities').map((c) => c.query.get('stage_id'))
    expect(stageQueries.sort()).toEqual(['s-lost', 's-neg', 's-open'])
  })

  it('moves a card with the keyboard-accessible select', async () => {
    const api = mockApi(routes())
    const { wrapper } = await mountView(PipelineBoardView, { path: '/pipelines?type=opportunity', pattern: '/pipelines' })
    await settle()
    const select = wrapper.find('[data-testid="card-o1"] select[data-testid="move-to"]')
    expect(select.attributes('aria-label')).toBe('Move Container order to stage')
    await select.setValue('s-neg')
    await settle()
    const patch = api.calls.find((c) => c.method === 'PATCH')
    expect(patch).toMatchObject({ path: '/opportunities/o1/stage', body: { stage_id: 's-neg' } })
    expect(api.calls.some((c) => c.method === 'PUT')).toBe(false)
    // The board reloads after the move.
    expect(api.calls.filter((c) => c.path === '/opportunities' && c.query.get('stage_id') === 's-open').length).toBeGreaterThan(1)
  })

  it('moves a card by drag and drop and records the outcome reason for lost stages', async () => {
    const api = mockApi(routes())
    const { wrapper } = await mountView(PipelineBoardView, { path: '/pipelines', pattern: '/pipelines' })
    await settle()
    await wrapper.find('[data-testid="card-o1"]').trigger('dragstart')
    const lost = wrapper.find('[data-testid="column-s-lost"]')
    await lost.trigger('dragover')
    await lost.trigger('drop')
    await settle()
    expect(api.calls.some((c) => c.method === 'PATCH')).toBe(false)
    expect(document.body.textContent).toContain('Mark “Container order” as lost')
    const reason = document.querySelector('[data-testid="outcome-reason"]') as HTMLTextAreaElement
    reason.value = 'Price too high'
    reason.dispatchEvent(new Event('input'))
    ;(document.querySelector('[data-testid="outcome-confirm"]') as HTMLButtonElement).click()
    await settle()
    expect(api.calls.find((c) => c.method === 'PATCH')).toMatchObject({ path: '/opportunities/o1/stage', body: { stage_id: 's-lost' } })
    const put = api.calls.find((c) => c.method === 'PUT')!
    expect(put.path).toBe('/opportunities/o1')
    expect(put.body).toEqual({
      name: 'Container order', company_id: null, supplier_id: null, contact_id: null, pipeline_stage_id: 's-lost', amount: '1000.00', currency: 'EUR',
      expected_close_date: null, probability: 0, outcome_reason: 'Price too high', description: 'desc', owner_id: null, custom_fields: { ref: 'X1' },
    })
  })

  it('uses the lead list endpoint for lead pipelines and hides moving without write access', async () => {
    const leadStages = [{ ...stages[0]!, id: 'l1', pipeline_id: 'lp' }, { ...stages[1]!, id: 'l2', pipeline_id: 'lp' }]
    const api = mockApi({
      ...baseRoutes,
      'GET /pipelines': (c) => (c.query.get('entity_type') === 'lead' ? { items: [{ ...pipelines.items[0]!, id: 'lp', entity_type: 'lead', stages: leadStages }] } : pipelines),
      'GET /leads': (c) => (c.query.get('stage_id') === 'l1' ? page([{ id: 'lead1', title: 'Website enquiry', currency: 'EUR', estimated_value: null, tags: [], source: 'website' }]) : page([])),
    })
    const { wrapper } = await mountView(PipelineBoardView, { path: '/pipelines?type=lead', pattern: '/pipelines', permissions: ['crm.read'] })
    await settle()
    expect(api.calls.some((c) => c.path === '/pipelines/lp/summary')).toBe(false)
    expect(wrapper.find('[data-testid="card-lead1"]').text()).toContain('Website enquiry')
    expect(wrapper.find('[data-testid="move-to"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="card-lead1"]').attributes('draggable')).toBe('false')
  })
})
