<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { ArrowRightLeft, Pencil, Trash2 } from 'lucide-vue-next'
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
import LeadFormModal from '@/components/crm/LeadFormModal.vue'
import ConvertLeadDialog from '@/components/crm/ConvertLeadDialog.vue'
import { customFields, useCustomFieldDefs, useDirectory, usePipelines } from '@/components/crm/crm'
import { moveToStage } from '@/components/crm/stageMove'
import { useAuth } from '@/stores/auth'
import { useSettings } from '@/stores/settings'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { formatDateTime, formatMoney, titleCase } from '@/lib/format'

type Lead = Schemas['LeadView']

const props = defineProps<{ id: string }>()
const auth = useAuth()
const router = useRouter()
const qc = useQueryClient()
const toast = useToast()
const { confirm } = useConfirm()
const { label } = useSettings()
const { nameOf } = useDirectory()
const { defs } = useCustomFieldDefs('lead')
const { stage, pipelineOf } = usePipelines('lead')

const key = computed(() => ['lead', props.id])
const q = useQuery({ queryKey: key, queryFn: ({ signal }) => api.get<Lead>(`/leads/${props.id}`, undefined, signal) })
const l = computed(() => q.data.value)
const notFound = computed(() => q.error.value instanceof ApiError && q.error.value.status === 404)
const canWrite = computed(() => auth.can('crm.write'))

const facts = computed<Fact[]>(() => {
  const x = l.value
  if (!x) return []
  return [
    { label: 'Source', value: titleCase(x.source) },
    { label: 'Estimated value', value: x.estimated_value ? formatMoney(x.estimated_value, x.currency) : null },
    { label: 'Pipeline', value: pipelineOf(x.pipeline_stage_id)?.name ?? null },
    { label: label('company'), slot: 'company' },
    { label: label('contact'), slot: 'contact' },
    { label: 'Owner', value: x.owner_name ?? nameOf(x.owner_id) ?? (x.owner_id ? 'Unknown user' : null) },
    { label: 'Created', value: formatDateTime(x.created_at) },
    { label: 'Updated', value: formatDateTime(x.updated_at) },
  ]
})

const editing = ref(false)
const converting = ref(false)
const moving = ref(false)
const stageKey = ref(0)

async function changeStage(stageId: string | null) {
  if (!l.value || !stageId || stageId === l.value.pipeline_stage_id) return
  moving.value = true
  try {
    await moveToStage('lead', props.id, stageId)
    toast.success(`Moved to ${stage(stageId)?.name ?? 'the new stage'}.`)
    await Promise.all([q.refetch(), qc.invalidateQueries({ queryKey: ['leads'] }), qc.invalidateQueries({ queryKey: ['board'] })])
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    moving.value = false
    stageKey.value++
  }
}
function saved() {
  void q.refetch()
  void qc.invalidateQueries({ queryKey: ['leads'] })
}
function setTags(tags: Schemas['TagRef'][]) {
  if (l.value) qc.setQueryData(key.value, { ...l.value, tags })
}
async function converted(doc: Schemas['ConvertLeadDoc']) {
  await Promise.all([qc.invalidateQueries({ queryKey: ['leads'] }), qc.invalidateQueries({ queryKey: ['opportunities'] }), qc.invalidateQueries({ queryKey: key.value })])
  await router.push(`/opportunities/${doc.opportunity.id}`)
}
async function remove() {
  if (!l.value) return
  const ok = await confirm({
    title: `Delete ${label('lead').toLowerCase()} “${l.value.title}”?`,
    message: 'Its notes, files and tags are deleted; linked tasks are kept without the link. This cannot be undone.',
    confirmLabel: 'Delete',
    danger: true,
  })
  if (!ok) return
  try {
    await api.del(`/leads/${props.id}`)
    toast.success(`${label('lead')} deleted.`)
    await qc.invalidateQueries({ queryKey: ['leads'] })
    await router.push('/leads')
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
</script>

<template>
  <div>
    <StateViews v-if="q.isLoading.value" kind="loading" />
    <div v-else-if="notFound" class="card">
      <StateViews kind="empty" :title="`${label('lead')} not found`" message="It may have been deleted.">
        <RouterLink to="/leads" class="text-sm text-brand-700 hover:underline">Back to {{ label('lead', true).toLowerCase() }}</RouterLink>
      </StateViews>
    </div>
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <template v-else-if="l">
      <PageHeader :title="l.title" back="/leads">
        <template #meta>
          <div class="mt-1 flex flex-wrap items-center gap-2">
            <UiBadge v-if="l.stage_name" :color="l.stage_color || undefined">{{ l.stage_name }}</UiBadge>
            <UiBadge v-if="l.converted_at" tone="green">converted</UiBadge>
          </div>
        </template>
        <template #actions>
          <UiButton v-if="canWrite && !l.converted_at" variant="primary" @click="converting = true"><ArrowRightLeft class="h-4 w-4" /> Convert to {{ label('opportunity').toLowerCase() }}</UiButton>
          <UiButton v-if="canWrite" @click="editing = true"><Pencil class="h-4 w-4" /> Edit</UiButton>
          <UiButton v-if="auth.can('crm.delete')" variant="danger" @click="remove"><Trash2 class="h-4 w-4" /> Delete</UiButton>
        </template>
      </PageHeader>

      <div v-if="l.converted_at" class="mb-4 rounded-lg border border-green-200 bg-green-50 px-4 py-3 text-sm text-green-800" role="status">
        Converted on {{ formatDateTime(l.converted_at) }}.
        <RouterLink v-if="l.converted_opportunity_id" :to="`/opportunities/${l.converted_opportunity_id}`" class="font-medium underline">Open the {{ label('opportunity').toLowerCase() }}</RouterLink>
      </div>

      <div class="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <div class="min-w-0 space-y-4 lg:col-span-2">
          <section class="card p-4" aria-label="Key facts">
            <DetailFacts :facts="facts">
              <template #company>
                <RecordLink type="company" :id="l.company_id" />
              </template>
              <template #contact>
                <RecordLink type="contact" :id="l.contact_id" />
              </template>
            </DetailFacts>
          </section>
          <section v-if="l.description" class="card p-4" aria-labelledby="lead-desc">
            <h2 id="lead-desc" class="mb-2 text-sm font-semibold text-slate-800">Description</h2>
            <p class="text-sm whitespace-pre-line text-slate-700">{{ l.description }}</p>
          </section>
          <section v-if="defs.length" class="card p-4" aria-labelledby="lead-cf">
            <h2 id="lead-cf" class="mb-3 text-sm font-semibold text-slate-800">Custom fields</h2>
            <CustomFieldsEditor :model-value="customFields(l.custom_fields)" entity-type="lead" readonly />
          </section>
        </div>
        <div class="min-w-0 space-y-4">
          <section v-if="canWrite" class="card p-4">
            <FormField v-slot="{ id }" label="Stage" :hint="moving ? 'Saving…' : undefined">
              <StageSelect :key="stageKey" :model-value="l.pipeline_stage_id ?? null" entity-type="lead" :input-id="id" :none-label="l.pipeline_stage_id ? null : 'Choose a stage…'" :disabled="moving" data-testid="lead-stage" @update:model-value="changeStage" />
            </FormField>
          </section>
          <section class="card p-4" aria-labelledby="lead-tags">
            <h2 id="lead-tags" class="mb-2 text-sm font-semibold text-slate-800">Tags</h2>
            <TagEditor entity-type="lead" :entity-id="l.id" :tags="l.tags" :can-edit="canWrite" @updated="setTags" />
          </section>
        </div>
      </div>

      <div class="mt-6"><RecordPanels entity-type="lead" :entity-id="l.id" /></div>

      <LeadFormModal :open="editing" :lead="l" @close="editing = false" @saved="saved" />
      <ConvertLeadDialog :open="converting" :lead="l" @close="converting = false" @converted="converted" />
    </template>
  </div>
</template>
