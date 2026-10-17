<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { Pencil, ThumbsDown, ThumbsUp, Trash2 } from 'lucide-vue-next'
import { api, ApiError, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import StateViews from '@/components/ui/StateViews.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import FormField from '@/components/ui/FormField.vue'
import TagEditor from '@/components/record/TagEditor.vue'
import CustomFieldsEditor from '@/components/record/CustomFieldsEditor.vue'
import RecordPanels from '@/components/record/RecordPanels.vue'
import DetailFacts, { type Fact } from '@/components/crm/DetailFacts.vue'
import RecordLink from '@/components/crm/RecordLink.vue'
import StageSelect from '@/components/crm/StageSelect.vue'
import OutcomeDialog from '@/components/crm/OutcomeDialog.vue'
import OpportunityProducts from '@/components/crm/OpportunityProducts.vue'
import OpportunityFormModal from '@/components/crm/OpportunityFormModal.vue'
import { customFields, formatDay, useCustomFieldDefs, useDirectory, usePipelines, type PipelineStage } from '@/components/crm/crm'
import { moveToStage } from '@/components/crm/stageMove'
import { useAuth } from '@/stores/auth'
import { useSettings } from '@/stores/settings'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { formatDateTime, formatMoney, titleCase } from '@/lib/format'

type Opportunity = Schemas['OpportunityView']

const props = defineProps<{ id: string }>()
const auth = useAuth()
const router = useRouter()
const qc = useQueryClient()
const toast = useToast()
const { confirm } = useConfirm()
const { label } = useSettings()
const { nameOf } = useDirectory()
const { defs } = useCustomFieldDefs('opportunity')
const { stage, pipelineOf } = usePipelines('opportunity')

const key = computed(() => ['opportunity', props.id])
const q = useQuery({ queryKey: key, queryFn: ({ signal }) => api.get<Opportunity>(`/opportunities/${props.id}`, undefined, signal) })
const o = computed(() => q.data.value)
const notFound = computed(() => q.error.value instanceof ApiError && q.error.value.status === 404)
const canWrite = computed(() => auth.can('crm.write'))
const pipeline = computed(() => pipelineOf(o.value?.pipeline_stage_id))
const wonStage = computed(() => pipeline.value?.stages.find((s) => s.category === 'won'))
const lostStage = computed(() => pipeline.value?.stages.find((s) => s.category === 'lost'))

const facts = computed<Fact[]>(() => {
  const x = o.value
  if (!x) return []
  return [
    { label: 'Amount', value: x.amount ? formatMoney(x.amount, x.currency) : null },
    { label: 'Probability', value: x.probability !== null && x.probability !== undefined ? `${x.probability}%` : null },
    { label: 'Expected close', value: x.expected_close_date ? formatDay(x.expected_close_date) : null },
    { label: 'Pipeline', value: pipeline.value?.name ?? null },
    { label: label('company'), slot: 'company' },
    { label: label('supplier'), slot: 'supplier' },
    { label: label('contact'), slot: 'contact' },
    ...(x.lead_id ? [{ label: `From ${label('lead').toLowerCase()}`, slot: 'lead' }] : []),
    { label: 'Owner', value: x.owner_name ?? nameOf(x.owner_id) ?? (x.owner_id ? 'Unknown user' : null) },
    ...(x.closed_at ? [{ label: 'Closed', value: formatDateTime(x.closed_at) }] : []),
    { label: 'Created', value: formatDateTime(x.created_at) },
    { label: 'Updated', value: formatDateTime(x.updated_at) },
  ]
})

const editing = ref(false)
const moving = ref(false)
const outcome = ref<PipelineStage | null>(null)
// Re-renders the stage select so it shows the stored stage after a cancelled or failed move.
const stageKey = ref(0)
function cancelOutcome() {
  outcome.value = null
  stageKey.value++
}

function requestStage(stageId: string | null) {
  if (!o.value || !stageId || stageId === o.value.pipeline_stage_id) return
  const st = stage(stageId)
  if (!st) return
  if (st.category === 'won' || st.category === 'lost') outcome.value = st
  else void applyStage(st, null)
}
async function applyStage(st: PipelineStage, reason: string | null) {
  outcome.value = null
  moving.value = true
  try {
    await moveToStage('opportunity', props.id, st.id, reason)
    toast.success(st.category === 'open' ? `Moved to ${st.name}.` : `Marked as ${st.category}.`)
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    moving.value = false
    await Promise.all([q.refetch(), qc.invalidateQueries({ queryKey: ['opportunities'] }), qc.invalidateQueries({ queryKey: ['board'] })])
    stageKey.value++
  }
}
function saved() {
  void q.refetch()
  void qc.invalidateQueries({ queryKey: ['opportunities'] })
}
function setTags(tags: Schemas['TagRef'][]) {
  if (o.value) qc.setQueryData(key.value, { ...o.value, tags })
}
async function remove() {
  if (!o.value) return
  const ok = await confirm({
    title: `Delete ${label('opportunity').toLowerCase()} “${o.value.name}”?`,
    message: 'Its product lines, notes, files and tags are deleted; linked tasks are kept without the link. This cannot be undone.',
    confirmLabel: 'Delete',
    danger: true,
  })
  if (!ok) return
  try {
    await api.del(`/opportunities/${props.id}`)
    toast.success(`${label('opportunity')} deleted.`)
    await qc.invalidateQueries({ queryKey: ['opportunities'] })
    await router.push('/opportunities')
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
</script>

<template>
  <div>
    <StateViews v-if="q.isLoading.value" kind="loading" />
    <div v-else-if="notFound" class="card">
      <StateViews kind="empty" :title="`${label('opportunity')} not found`" message="It may have been deleted.">
        <RouterLink to="/opportunities" class="text-sm text-brand-700 hover:underline">Back to {{ label('opportunity', true).toLowerCase() }}</RouterLink>
      </StateViews>
    </div>
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <template v-else-if="o">
      <PageHeader :title="o.name" back="/opportunities">
        <template #meta>
          <div class="mt-1 flex flex-wrap items-center gap-2">
            <UiBadge :value="o.status" />
            <UiBadge v-if="o.stage_name" :color="o.stage_color || undefined">{{ o.stage_name }}</UiBadge>
          </div>
        </template>
        <template #actions>
          <template v-if="canWrite">
            <UiButton v-if="wonStage && o.status !== 'won'" :disabled="moving" @click="requestStage(wonStage.id)"><ThumbsUp class="h-4 w-4" /> Mark won</UiButton>
            <UiButton v-if="lostStage && o.status !== 'lost'" :disabled="moving" @click="requestStage(lostStage.id)"><ThumbsDown class="h-4 w-4" /> Mark lost</UiButton>
            <UiButton @click="editing = true"><Pencil class="h-4 w-4" /> Edit</UiButton>
          </template>
          <UiButton v-if="auth.can('crm.delete')" variant="danger" @click="remove"><Trash2 class="h-4 w-4" /> Delete</UiButton>
        </template>
      </PageHeader>

      <div v-if="o.status !== 'open'" class="mb-4 rounded-lg border px-4 py-3 text-sm" :class="o.status === 'won' ? 'border-green-200 bg-green-50 text-green-800' : 'border-red-200 bg-red-50 text-red-800'" role="status">
        {{ titleCase(o.status) }}<template v-if="o.closed_at"> on {{ formatDateTime(o.closed_at) }}</template>.
        <template v-if="o.outcome_reason"> Reason: {{ o.outcome_reason }}</template>
      </div>

      <div class="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <div class="min-w-0 space-y-4 lg:col-span-2">
          <section class="card p-4" aria-label="Key facts">
            <DetailFacts :facts="facts">
              <template #company><RecordLink type="company" :id="o.company_id" :name="o.company_name" /></template>
              <template #supplier><RecordLink type="supplier" :id="o.supplier_id" :name="o.supplier_name" /></template>
              <template #contact><RecordLink type="contact" :id="o.contact_id" :name="o.contact_name" /></template>
              <template #lead><RecordLink type="lead" :id="o.lead_id" /></template>
            </DetailFacts>
          </section>
          <OpportunityProducts :opportunity-id="o.id" :currency="o.currency" />
          <section v-if="o.description" class="card p-4" aria-labelledby="opp-desc">
            <h2 id="opp-desc" class="mb-2 text-sm font-semibold text-slate-800">Description</h2>
            <p class="text-sm whitespace-pre-line text-slate-700">{{ o.description }}</p>
          </section>
          <section v-if="defs.length" class="card p-4" aria-labelledby="opp-cf">
            <h2 id="opp-cf" class="mb-3 text-sm font-semibold text-slate-800">Custom fields</h2>
            <CustomFieldsEditor :model-value="customFields(o.custom_fields)" entity-type="opportunity" readonly />
          </section>
        </div>
        <div class="min-w-0 space-y-4">
          <section v-if="canWrite" class="card p-4">
            <FormField v-slot="{ id }" label="Stage" :hint="moving ? 'Saving…' : 'Won and lost stages ask for a reason.'">
              <StageSelect :key="stageKey" :model-value="o.pipeline_stage_id ?? null" entity-type="opportunity" :input-id="id" :none-label="o.pipeline_stage_id ? null : 'Choose a stage…'" :disabled="moving" @update:model-value="requestStage" />
            </FormField>
          </section>
          <section class="card p-4" aria-labelledby="opp-tags">
            <h2 id="opp-tags" class="mb-2 text-sm font-semibold text-slate-800">Tags</h2>
            <TagEditor entity-type="opportunity" :entity-id="o.id" :tags="o.tags" :can-edit="canWrite" @updated="setTags" />
          </section>
        </div>
      </div>

      <div class="mt-6"><RecordPanels entity-type="opportunity" :entity-id="o.id" /></div>

      <OpportunityFormModal :open="editing" :opportunity="o" @close="editing = false" @saved="saved" />
      <OutcomeDialog
        :open="!!outcome"
        :category="outcome?.category ?? ''"
        :stage-name="outcome?.name ?? ''"
        :record-name="o.name"
        :initial="o.outcome_reason"
        @cancel="cancelOutcome"
        @confirm="(reason) => outcome && applyStage(outcome, reason)"
      />
    </template>
  </div>
</template>
