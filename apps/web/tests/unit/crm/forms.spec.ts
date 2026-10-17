import { afterEach, describe, expect, it, vi } from 'vitest'
import CompanyFormModal from '@/components/crm/CompanyFormModal.vue'
import LeadFormModal from '@/components/crm/LeadFormModal.vue'
import OpportunityFormModal from '@/components/crm/OpportunityFormModal.vue'
import { apiError, mockApi, mountView, settle } from '../helpers'
import { USER_ID, baseRoutes, company } from './fixtures'

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

const pipelines = {
  items: [
    {
      id: 'p1', name: 'Sales', entity_type: 'opportunity', is_default: true, position: 0, created_at: '', updated_at: '',
      stages: [
        { id: 's1', name: 'Qualify', category: 'open', color: '#111111', position: 0, pipeline_id: 'p1', probability: 10, created_at: '' },
        { id: 's2', name: 'Won', category: 'won', color: '#00aa00', position: 1, pipeline_id: 'p1', probability: 100, created_at: '' },
      ],
    },
  ],
}

function field(label: string): HTMLElement {
  const l = [...document.querySelectorAll('label')].find((x) => x.textContent?.replace('*', '').trim() === label)
  if (!l) throw new Error(`No field ${label}`)
  return document.getElementById(l.htmlFor) as HTMLElement
}
function type(label: string, value: string) {
  const el = field(label) as HTMLInputElement
  el.value = value
  el.dispatchEvent(new Event('input'))
  el.dispatchEvent(new Event('change'))
}
async function submit(id: string) {
  document.querySelector<HTMLFormElement>(`#${id}`)!.dispatchEvent(new Event('submit'))
  await settle()
}

describe('CompanyFormModal', () => {
  it('shows server validation errors next to the fields', async () => {
    const api = mockApi({
      ...baseRoutes,
      'POST /companies': () => apiError(422, 'validation_failed', 'Validation failed', { name: 'must be between 1 and 200 characters', email: 'must be a valid e-mail address' }),
    })
    const { wrapper } = await mountView(CompanyFormModal, { props: { open: true } })
    type('E-mail', 'not-an-email')
    await submit('company-form')
    const post = api.calls.find((c) => c.method === 'POST')!
    expect(post.body).toMatchObject({ name: '', email: 'not-an-email', company_type: 'prospect', status: 'active', owner_id: USER_ID, custom_fields: {} })
    expect(post.body).not.toHaveProperty('tag_ids')
    expect(wrapper.findComponent(CompanyFormModal).emitted('saved')).toBeUndefined()
    const nameInput = field('Name')
    const error = document.getElementById(`${nameInput.id}-error`)
    expect(error?.textContent).toBe('must be between 1 and 200 characters')
    expect(document.getElementById(`${field('E-mail').id}-error`)?.textContent).toBe('must be a valid e-mail address')
  })

  it('sends exactly the CompanyInput fields and tag ids on create', async () => {
    const api = mockApi({ ...baseRoutes, 'POST /companies': (c) => ({ ...company('new', 'Acme'), ...(c.body as object) }) })
    const { wrapper } = await mountView(CompanyFormModal, { props: { open: true } })
    type('Name', 'Acme')
    type('Employees', '25')
    type('Website', 'acme.test')
    const tags = field('Tags') as HTMLSelectElement
    await settle()
    tags.value = 'tag-1'
    tags.dispatchEvent(new Event('change'))
    await submit('company-form')
    const body = api.calls.find((c) => c.method === 'POST')!.body as Record<string, unknown>
    expect(Object.keys(body).sort()).toEqual(
      ['address', 'city', 'company_type', 'country', 'custom_fields', 'description', 'domain', 'email', 'employee_count', 'industry', 'name', 'owner_id', 'phone', 'status', 'tag_ids', 'website'].sort(),
    )
    expect(body).toMatchObject({ name: 'Acme', employee_count: 25, website: 'acme.test', domain: null, tag_ids: ['tag-1'] })
    expect(wrapper.findComponent(CompanyFormModal).emitted('saved')).toHaveLength(1)
  })
})

describe('LeadFormModal', () => {
  it('rejects a malformed amount before calling the API and defaults the currency', async () => {
    const api = mockApi({ ...baseRoutes, 'GET /pipelines': { items: [] } })
    await mountView(LeadFormModal, { props: { open: true } })
    await settle()
    expect((field('Currency') as HTMLInputElement).value).toBe('EUR')
    type('Title', 'Big order')
    type('Estimated value', '12,5.0x')
    await submit('lead-form')
    expect(api.calls.some((c) => c.method === 'POST')).toBe(false)
    expect(document.getElementById(`${field('Estimated value').id}-error`)?.textContent).toContain('must be a number')
  })
})

describe('OpportunityFormModal', () => {
  it('posts money as a decimal string and asks for a reason on won stages', async () => {
    const api = mockApi({ ...baseRoutes, 'GET /pipelines': pipelines, 'POST /opportunities': (c) => ({ id: 'o1', ...(c.body as object) }) })
    await mountView(OpportunityFormModal, { props: { open: true } })
    await settle()
    type('Name', 'Container deal')
    type('Amount', '1250.50')
    type('Expected close date', '2026-12-01')
    expect([...document.querySelectorAll('label')].some((l) => l.textContent?.includes('Reason won'))).toBe(false)
    const stage = field('Pipeline stage') as HTMLSelectElement
    stage.value = 's2'
    stage.dispatchEvent(new Event('change'))
    await settle()
    type('Reason won', 'Best price')
    await submit('opportunity-form')
    const body = api.calls.find((c) => c.method === 'POST')!.body as Record<string, unknown>
    expect(body).toMatchObject({ name: 'Container deal', amount: '1250.50', currency: 'EUR', expected_close_date: '2026-12-01', pipeline_stage_id: 's2', outcome_reason: 'Best price', probability: null })
  })
})
