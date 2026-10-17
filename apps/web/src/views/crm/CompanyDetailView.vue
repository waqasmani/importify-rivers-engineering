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
import TagEditor from '@/components/record/TagEditor.vue'
import CustomFieldsEditor from '@/components/record/CustomFieldsEditor.vue'
import RecordPanels from '@/components/record/RecordPanels.vue'
import DetailFacts, { type Fact } from '@/components/crm/DetailFacts.vue'
import RelatedRecords from '@/components/crm/RelatedRecords.vue'
import MergeDialog from '@/components/crm/MergeDialog.vue'
import CompanyFormModal from '@/components/crm/CompanyFormModal.vue'
import ContactFormModal from '@/components/crm/ContactFormModal.vue'
import LeadFormModal from '@/components/crm/LeadFormModal.vue'
import OpportunityFormModal from '@/components/crm/OpportunityFormModal.vue'
import SupplierFormModal from '@/components/crm/SupplierFormModal.vue'
import { customFields, useCustomFieldDefs, useDirectory } from '@/components/crm/crm'
import { useAuth } from '@/stores/auth'
import { useSettings } from '@/stores/settings'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { formatDateTime, titleCase } from '@/lib/format'

type Company = Schemas['CompanyView']

const props = defineProps<{ id: string }>()
const auth = useAuth()
const router = useRouter()
const qc = useQueryClient()
const toast = useToast()
const { confirm } = useConfirm()
const { label } = useSettings()
const { nameOf } = useDirectory()
const { defs } = useCustomFieldDefs('company')

const key = computed(() => ['company', props.id])
const q = useQuery({ queryKey: key, queryFn: ({ signal }) => api.get<Company>(`/companies/${props.id}`, undefined, signal) })
const c = computed(() => q.data.value)
const notFound = computed(() => q.error.value instanceof ApiError && q.error.value.status === 404)

const facts = computed<Fact[]>(() => {
  const x = c.value
  if (!x) return []
  return [
    { label: 'Type', value: titleCase(x.company_type) },
    { label: 'Website', value: x.website, href: x.website ?? undefined },
    { label: 'Domain', value: x.domain },
    { label: 'Industry', value: x.industry },
    { label: 'Employees', value: x.employee_count },
    { label: 'E-mail', value: x.email, href: x.email ? `mailto:${x.email}` : undefined },
    { label: 'Phone', value: x.phone, href: x.phone ? `tel:${x.phone}` : undefined },
    { label: 'Location', value: [x.city, x.country].filter(Boolean).join(', ') },
    { label: 'Address', value: x.address },
    { label: 'Owner', value: x.owner_name ?? nameOf(x.owner_id) ?? (x.owner_id ? 'Unknown user' : null) },
    { label: 'Created', value: formatDateTime(x.created_at) },
    { label: 'Updated', value: formatDateTime(x.updated_at) },
  ]
})

const editing = ref(false)
const merging = ref(false)
const adding = ref<'contact' | 'lead' | 'opportunity' | 'supplier' | null>(null)
const preset = computed(() => ({ company_id: props.id, company_name: c.value?.name ?? null }))

function saved(next: Company) {
  qc.setQueryData(key.value, next)
  void qc.invalidateQueries({ queryKey: ['companies'] })
}
function setTags(tags: Schemas['TagRef'][]) {
  if (c.value) qc.setQueryData(key.value, { ...c.value, tags })
}
async function relatedAdded() {
  await qc.invalidateQueries({ queryKey: ['related'] })
}
async function merged() {
  await Promise.all([q.refetch(), qc.invalidateQueries({ queryKey: ['related'] }), qc.invalidateQueries({ queryKey: ['companies'] }), qc.invalidateQueries({ queryKey: ['notes'] })])
}
async function remove() {
  if (!c.value) return
  const ok = await confirm({
    title: `Delete ${label('company').toLowerCase()} “${c.value.name}”?`,
    message: `Its notes, files and tags are deleted. Linked ${label('contact', true).toLowerCase()}, ${label('lead', true).toLowerCase()}, ${label('opportunity', true).toLowerCase()} and tasks are kept without the link. This cannot be undone.`,
    confirmLabel: 'Delete',
    danger: true,
  })
  if (!ok) return
  try {
    await api.del(`/companies/${props.id}`)
    toast.success(`${label('company')} deleted.`)
    await qc.invalidateQueries({ queryKey: ['companies'] })
    await router.push('/companies')
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
</script>

<template>
  <div>
    <StateViews v-if="q.isLoading.value" kind="loading" />
    <div v-else-if="notFound" class="card">
      <StateViews kind="empty" :title="`${label('company')} not found`" message="It may have been deleted or merged into another record.">
        <RouterLink to="/companies" class="text-sm text-brand-700 hover:underline">Back to {{ label('company', true).toLowerCase() }}</RouterLink>
      </StateViews>
    </div>
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <template v-else-if="c">
      <PageHeader :title="c.name" back="/companies">
        <template #meta>
          <div class="mt-1 flex flex-wrap items-center gap-2"><UiBadge :value="c.status" /><span class="text-sm text-slate-500">{{ titleCase(c.company_type) }}</span></div>
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
          <section v-if="c.description" class="card p-4" aria-labelledby="company-desc">
            <h2 id="company-desc" class="mb-2 text-sm font-semibold text-slate-800">Description</h2>
            <p class="text-sm whitespace-pre-line text-slate-700">{{ c.description }}</p>
          </section>
          <section v-if="defs.length" class="card p-4" aria-labelledby="company-cf">
            <h2 id="company-cf" class="mb-3 text-sm font-semibold text-slate-800">Custom fields</h2>
            <CustomFieldsEditor :model-value="customFields(c.custom_fields)" entity-type="company" readonly />
          </section>
          <RelatedRecords :title="label('contact', true)" type="contact" endpoint="/contacts" :query="{ company_id: c.id }" :view-all="`/contacts?company_id=${c.id}`">
            <template #actions>
              <UiButton v-if="auth.can('crm.write')" size="sm" @click="adding = 'contact'"><Plus class="h-3.5 w-3.5" /> Add</UiButton>
            </template>
          </RelatedRecords>
          <RelatedRecords :title="label('opportunity', true)" type="opportunity" endpoint="/opportunities" :query="{ company_id: c.id }" :view-all="`/opportunities?company_id=${c.id}`">
            <template #actions>
              <UiButton v-if="auth.can('crm.write')" size="sm" @click="adding = 'opportunity'"><Plus class="h-3.5 w-3.5" /> Add</UiButton>
            </template>
          </RelatedRecords>
        </div>
        <div class="min-w-0 space-y-4">
          <section class="card p-4" aria-labelledby="company-tags">
            <h2 id="company-tags" class="mb-2 text-sm font-semibold text-slate-800">Tags</h2>
            <TagEditor entity-type="company" :entity-id="c.id" :tags="c.tags" :can-edit="auth.can('crm.write')" @updated="setTags" />
          </section>
          <RelatedRecords :title="label('lead', true)" type="lead" endpoint="/leads" :query="{ company_id: c.id }" :view-all="`/leads?company_id=${c.id}`">
            <template #actions>
              <UiButton v-if="auth.can('crm.write')" size="sm" @click="adding = 'lead'"><Plus class="h-3.5 w-3.5" /> Add</UiButton>
            </template>
          </RelatedRecords>
          <RelatedRecords :title="label('supplier', true)" type="supplier" endpoint="/suppliers" :match="(r) => r.company_id === c!.id">
            <template #actions>
              <UiButton v-if="auth.can('crm.write')" size="sm" @click="adding = 'supplier'"><Plus class="h-3.5 w-3.5" /> Add</UiButton>
            </template>
          </RelatedRecords>
        </div>
      </div>

      <div class="mt-6"><RecordPanels entity-type="company" :entity-id="c.id" /></div>

      <CompanyFormModal :open="editing" :company="c" @close="editing = false" @saved="saved" />
      <MergeDialog :open="merging" entity-type="company" :record="c" :record-label="c.name" @close="merging = false" @merged="merged" />
      <ContactFormModal :open="adding === 'contact'" :preset="preset" @close="adding = null" @saved="relatedAdded" />
      <LeadFormModal :open="adding === 'lead'" :preset="preset" @close="adding = null" @saved="relatedAdded" />
      <OpportunityFormModal :open="adding === 'opportunity'" :preset="preset" @close="adding = null" @saved="relatedAdded" />
      <SupplierFormModal :open="adding === 'supplier'" :preset="preset" @close="adding = null" @saved="relatedAdded" />
    </template>
  </div>
</template>
