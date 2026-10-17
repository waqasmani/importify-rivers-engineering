import { afterEach, describe, expect, it, vi } from 'vitest'
import PipelineFormModal from '@/components/settings/PipelineFormModal.vue'
import { mockApi, mountView, settle } from '../helpers'

const PID = '55555555-5555-4555-8555-555555555555'
const stage = (id: string, name: string, position: number, category = 'open') => ({
  id, name, position, category, color: '#64748b', probability: null, pipeline_id: PID, created_at: '',
})
const pipeline = {
  id: PID, entity_type: 'opportunity', name: 'Sales', is_default: true, position: 0, created_at: '', updated_at: '',
  stages: [stage('s1', 'Qualified', 0), stage('s2', 'Proposal', 1), stage('s3', 'Won', 2, 'won')],
}

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

describe('PipelineFormModal', () => {
  it('reorders, removes and adds stages, keeping ids of existing stages', async () => {
    const api = mockApi({ [`PUT /pipelines/${PID}`]: (c) => ({ ...pipeline, ...(c.body as object) }) })
    const { wrapper } = await mountView(PipelineFormModal, { props: { open: true, pipeline, entityType: 'opportunity', entityLabel: 'Opportunity', nextPosition: 1 } })
    await wrapper.find('button[aria-label="Move Proposal up"]').trigger('click')
    await wrapper.find('button[aria-label="Remove Won"]').trigger('click')
    expect(wrapper.text()).toContain('Removing Won leaves any records in that stage without a stage')
    await wrapper.findAll('button').find((b) => b.text().includes('Add stage'))!.trigger('click')
    const rows = wrapper.findAll('[data-testid="stage-row"]')
    expect(rows).toHaveLength(3)
    await rows[2]!.find('input').setValue('Closed')
    await rows[2]!.find('select').setValue('lost')
    await rows[2]!.find('input[type="number"]').setValue('0')
    await wrapper.find('form').trigger('submit')
    await settle()
    const body = api.calls.find((c) => c.method === 'PUT')!.body as { stages: object[]; is_default: boolean; entity_type: string }
    expect(body.entity_type).toBe('opportunity')
    expect(body.stages).toEqual([
      { id: 's2', name: 'Proposal', category: 'open', color: '#64748b', probability: null },
      { id: 's1', name: 'Qualified', category: 'open', color: '#64748b', probability: null },
      { id: null, name: 'Closed', category: 'lost', color: '#64748b', probability: 0 },
    ])
  })
})
