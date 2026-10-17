<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { CopyCheck, Pencil, Plus, Trash2 } from 'lucide-vue-next'
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
import { PLACEMENT_LABELS } from '@/components/orders/order-utils'
import RelatedRecords from '@/components/crm/RelatedRecords.vue'
import MergeDialog from '@/components/crm/MergeDialog.vue'
import StageSelect from '@/components/crm/StageSelect.vue'
import SupplierProducts from '@/components/crm/SupplierProducts.vue'
import SupplierFormModal from '@/components/crm/SupplierFormModal.vue'
import ContactFormModal from '@/components/crm/ContactFormModal.vue'
import OpportunityFormModal from '@/components/crm/OpportunityFormModal.vue'
import { customFields, useCustomFieldDefs, useDirectory, usePipelines } from '@/components/crm/crm'
import { useAuth } from '@/stores/auth'
import { useSettings } from '@/stores/settings'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { formatDateTime, titleCase } from '@/lib/format'

type Supplier = Schemas['SupplierView']

const props = defineProps<{ id: string }>()
const auth = useAuth()
const router = useRouter()
const qc = useQueryClient()
const toast = useToast()
const { confirm } = useConfirm()
const { label } = useSettings()
const { nameOf } = useDirectory()
const { defs } = useCustomFieldDefs('supplier')
const { stage } = usePipelines('supplier')

const key = computed(() => ['supplier', props.id])
const q = useQuery({ queryKey: key, queryFn: ({ signal }) => api.get<Supplier>(`/suppliers/${props.id}`, undefined, signal) })
const s = computed(() => q.data.value)
const notFound = computed(() => q.error.value instanceof ApiError && q.error.value.status === 404)

const facts = computed<Fact[]>(() => {
  const x = s.value
  if (!x) return []
  return [
    { label: 'Type', value: titleCase(x.supplier_type) },
    { label: 'Order placement', value: PLACEMENT_LABELS[x.order_placement] ?? x.order_placement },
    { label: 'Rating', value: x.rating ? `${x.rating} / 5` : null },
    { label: 'Website', value: x.website, href: x.website ?? undefined },
    { label: 'Domain', value: x.domain },
    { label: 'E-mail', value: x.email, href: x.email ? `mailto:${x.email}` : undefined },
    { label: 'Phone', value: x.phone, href: x.phone ? `tel:${x.phone}` : undefined },
    { label: 'Location', value: [x.city, x.country].filter(Boolean).join(', ') },
    { label: 'Address', value: x.address },
    { label: label('company'), value: x.company_id ? (x.company_name ?? 'Open') : null, to: x.company_id ? `/companies/${x.company_id}` : undefined },
    { label: `Primary ${label('contact').toLowerCase()}`, value: x.primary_contact_id ? (x.primary_contact_name ?? 'Open') : null, to: x.primary_contact_id ? `/contacts/${x.primary_contact_id}` : undefined },
    { label: 'Payment terms', value: x.payment_terms },
    { label: 'Lead time', value: x.lead_time_days !== null && x.lead_time_days !== undefined ? `${x.lead_time_days} days` : null },
    { label: 'Certifications', value: (x.certifications ?? []).join(', ') },
    { label: 'Owner', value: nameOf(x.owner_id) ?? (x.owner_id ? 'Unknown user' : null) },
    { label: 'Updated', value: formatDateTime(x.updated_at) },
  ]
})

const editing = ref(false)
const merging = ref(false)
const adding = ref<'contact' | 'opportunity' | null>(null)
const movingStage = ref(false)
const stageKey = ref(0)
const preset = computed(() => ({ supplier_id: props.id, supplier_name: s.value?.name ?? null }))

async function changeStage(stageId: string | null) {
  if (!s.value || stageId === (s.value.pipeline_stage_id ?? null)) return
  movingStage.value = true
  try {
    await api.patch(`/suppliers/${props.id}/stage`, { stage_id: stageId } satisfies Schemas['StageInput'])
    toast.success(stageId ? `Moved to ${stage(stageId)?.name ?? 'the new stage'}.` : 'Stage cleared.')
    await Promise.all([q.refetch(), qc.invalidateQueries({ queryKey: ['suppliers'] }), qc.invalidateQueries({ queryKey: ['board'] })])
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    movingStage.value = false
    stageKey.value++
  }
}
function saved() {
  void q.refetch()
  void qc.invalidateQueries({ queryKey: ['suppliers'] })
}
function setTags(tags: Schemas['TagRef'][]) {
  if (s.value) qc.setQueryData(key.value, { ...s.value, tags })
}
async function relatedAdded() {
  await qc.invalidateQueries({ queryKey: ['related'] })
}
async function merged() {
  await Promise.all([q.refetch(), qc.invalidateQueries({ queryKey: ['related'] }), qc.invalidateQueries({ queryKey: ['supplier-products', props.id] }), qc.invalidateQueries({ queryKey: ['suppliers'] }), qc.invalidateQueries({ queryKey: ['notes'] })])
}
async function remove() {
  if (!s.value) return
  const ok = await confirm({
    title: `Delete ${label('supplier').toLowerCase()} “${s.value.name}”?`,
    message: `Its notes, files, tags and ${label('product').toLowerCase()} links are deleted. ${label('product', true)}, ${label('contact', true).toLowerCase()} and tasks are kept without the link. This cannot be undone.`,
    confirmLabel: 'Delete',
    danger: true,
  })
  if (!ok) return
  try {
    await api.del(`/suppliers/${props.id}`)
    toast.success(`${label('supplier')} deleted.`)
    await qc.invalidateQueries({ queryKey: ['suppliers'] })
    await router.push('/suppliers')
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
</script>

<template>
  <div>
    <StateViews v-if="q.isLoading.value" kind="loading" />
    <div v-else-if="notFound" class="card">
      <StateViews kind="empty" :title="`${label('supplier')} not found`" message="It may have been deleted or merged into another record.">
        <RouterLink to="/suppliers" class="text-sm text-brand-700 hover:underline">Back to {{ label('supplier', true).toLowerCase() }}</RouterLink>
      </StateViews>
    </div>
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <template v-else-if="s">
      <PageHeader :title="s.name" back="/suppliers">
        <template #meta>
          <div class="mt-1 flex flex-wrap items-center gap-2">
            <UiBadge v-if="s.stage_name" :color="s.stage_color || undefined">{{ s.stage_name }}</UiBadge>
            <span class="text-sm text-slate-500">{{ titleCase(s.supplier_type) }}</span>
          </div>
        </template>
        <template #actions>
          <UiButton v-if="auth.can('crm.delete')" @click="merging = true"><CopyCheck class="h-4 w-4" /> Find duplicates</UiButton>
          <UiButton v-if="auth.can('crm.write')" @click="editing = true"><Pencil class="h-4 w-4" /> Edit</UiButton>
          <UiButton v-if="auth.can('crm.delete')" variant="danger" @click="remove"><Trash2 class="h-4 w-4" /> Delete</UiButton>
        </template>
      </PageHeader>

      <div class="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <div class="min-w-0 space-y-4 lg:col-span-2">
          <section class="card p-4" aria-label="Key facts"><DetailFacts :facts="facts" /></section>
          <section v-if="s.moq_notes || s.description" class="card space-y-3 p-4" aria-label="Notes">
            <div v-if="s.moq_notes">
              <h2 class="mb-1 text-sm font-semibold text-slate-800">MOQ notes</h2>
              <p class="text-sm whitespace-pre-line text-slate-700">{{ s.moq_notes }}</p>
            </div>
            <div v-if="s.description">
              <h2 class="mb-1 text-sm font-semibold text-slate-800">Description</h2>
              <p class="text-sm whitespace-pre-line text-slate-700">{{ s.description }}</p>
            </div>
          </section>
          <SupplierProducts :supplier-id="s.id" :supplier-name="s.name" />
          <section v-if="defs.length" class="card p-4" aria-labelledby="supplier-cf">
            <h2 id="supplier-cf" class="mb-3 text-sm font-semibold text-slate-800">Custom fields</h2>
            <CustomFieldsEditor :model-value="customFields(s.custom_fields)" entity-type="supplier" readonly />
          </section>
        </div>
        <div class="min-w-0 space-y-4">
          <section v-if="auth.can('crm.write')" class="card p-4">
            <FormField v-slot="{ id }" label="Pipeline stage">
              <StageSelect :key="stageKey" :model-value="s.pipeline_stage_id ?? null" entity-type="supplier" :input-id="id" none-label="No stage" :disabled="movingStage" @update:model-value="changeStage" />
            </FormField>
          </section>
          <section class="card p-4" aria-labelledby="supplier-tags">
            <h2 id="supplier-tags" class="mb-2 text-sm font-semibold text-slate-800">Tags</h2>
            <TagEditor entity-type="supplier" :entity-id="s.id" :tags="s.tags" :can-edit="auth.can('crm.write')" @updated="setTags" />
          </section>
          <RelatedRecords :title="label('contact', true)" type="contact" endpoint="/contacts" :query="{ supplier_id: s.id }" :view-all="`/contacts?supplier_id=${s.id}`">
            <template #actions>
              <UiButton v-if="auth.can('crm.write')" size="sm" @click="adding = 'contact'"><Plus class="h-3.5 w-3.5" /> Add</UiButton>
            </template>
          </RelatedRecords>
          <RelatedRecords :title="label('opportunity', true)" type="opportunity" endpoint="/opportunities" :query="{ supplier_id: s.id }" :view-all="`/opportunities?supplier_id=${s.id}`">
            <template #actions>
              <UiButton v-if="auth.can('crm.write')" size="sm" @click="adding = 'opportunity'"><Plus class="h-3.5 w-3.5" /> Add</UiButton>
            </template>
          </RelatedRecords>
        </div>
      </div>

      <div class="mt-6"><RecordPanels entity-type="supplier" :entity-id="s.id" /></div>

      <SupplierFormModal :open="editing" :supplier="s" @close="editing = false" @saved="saved" />
      <MergeDialog :open="merging" entity-type="supplier" :record="s" :record-label="s.name" @close="merging = false" @merged="merged" />
      <ContactFormModal :open="adding === 'contact'" :preset="preset" @close="adding = null" @saved="relatedAdded" />
      <OpportunityFormModal :open="adding === 'opportunity'" :preset="preset" @close="adding = null" @saved="relatedAdded" />
    </template>
  </div>
</template>
