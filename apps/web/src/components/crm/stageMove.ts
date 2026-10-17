// Moves a pipeline record to another stage with PATCH /<type>/{id}/stage. Opportunities moving to
// a won or lost stage can carry an outcome reason, which the stage endpoint does not accept, so it
// is saved with a follow-up PUT of the full opportunity.
import { api, type Schemas } from '@/api/client'
import { CRM_PATH, opportunityInput, type PipelineEntity } from './crm'

export async function moveToStage(type: PipelineEntity, id: string, stageId: string, reason?: string | null): Promise<void> {
  const body: Schemas['StageInput'] = { stage_id: stageId }
  const moved = await api.patch<Schemas['Opportunity'] | Schemas['Lead'] | Schemas['Supplier']>(`${CRM_PATH[type]}/${id}/stage`, body)
  if (type === 'opportunity' && reason) {
    const o = moved as Schemas['Opportunity']
    try {
      await api.put(`/opportunities/${id}`, opportunityInput(o, { outcome_reason: reason }))
    } catch (e) {
      throw new Error(`The stage was changed, but the outcome reason could not be saved: ${e instanceof Error ? e.message : String(e)}`)
    }
  }
}
