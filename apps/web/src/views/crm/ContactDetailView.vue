<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { Building2, CopyCheck, Pencil, Trash2, Truck } from 'lucide-vue-next'
import { api, ApiError, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import StateViews from '@/components/ui/StateViews.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import TagEditor from '@/components/record/TagEditor.vue'
import CustomFieldsEditor from '@/components/record/CustomFieldsEditor.vue'
import RecordPanels from '@/components/record/RecordPanels.vue'
import DetailFacts, { type Fact } from '@/components/crm/DetailFacts.vue'
import MergeDialog from '@/components/crm/MergeDialog.vue'
import ContactFormModal from '@/components/crm/ContactFormModal.vue'
import { contactName, customFields, useCustomFieldDefs, useDirectory } from '@/components/crm/crm'
import { useAuth } from '@/stores/auth'
import { useSettings } from '@/stores/settings'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { formatDateTime } from '@/lib/format'

type Contact = Schemas['ContactView']

const props = defineProps<{ id: string }>()
const auth = useAuth()
const router = useRouter()
const qc = useQueryClient()
const toast = useToast()
const { confirm } = useConfirm()
const { label } = useSettings()
const { nameOf } = useDirectory()
const { defs } = useCustomFieldDefs('contact')

const key = computed(() => ['contact', props.id])
const q = useQuery({ queryKey: key, queryFn: ({ signal }) => api.get<Contact>(`/contacts/${props.id}`, undefined, signal) })
const c = computed(() => q.data.value)
const name = computed(() => (c.value ? contactName(c.value) : ''))
const notFound = computed(() => q.error.value instanceof ApiError && q.error.value.status === 404)

const facts = computed<Fact[]>(() => {
  const x = c.value
  if (!x) return []
  return [
    { label: 'E-mail', value: x.email, href: x.email ? `mailto:${x.email}` : undefined },
    { label: 'Phone', value: x.phone, href: x.phone ? `tel:${x.phone}` : undefined },
    { label: 'Job title', value: x.job_title },
    { label: 'Decision maker', value: x.is_decision_maker ? 'Yes' : 'No' },
    { label: 'LinkedIn', value: x.linkedin_url ? 'Profile' : null, href: x.linkedin_url || undefined },
    { label: 'Country', value: x.country },
    { label: 'Owner', value: nameOf(x.owner_id) ?? (x.owner_id ? 'Unknown user' : null) },
    { label: 'Created', value: formatDateTime(x.created_at) },
    { label: 'Updated', value: formatDateTime(x.updated_at) },
  ]
})

const editing = ref(false)
const merging = ref(false)

// PUT returns the contact without the linked names, so reload the full view.
function saved() {
  void q.refetch()
  void qc.invalidateQueries({ queryKey: ['contacts'] })
}
function setTags(tags: Schemas['TagRef'][]) {
  if (c.value) qc.setQueryData(key.value, { ...c.value, tags })
}
async function merged() {
  await Promise.all([q.refetch(), qc.invalidateQueries({ queryKey: ['contacts'] }), qc.invalidateQueries({ queryKey: ['notes'] })])
}
async function remove() {
  if (!c.value) return
  const ok = await confirm({
    title: `Delete ${label('contact').toLowerCase()} “${name.value}”?`,
    message: 'Their notes, files and tags are deleted. Linked records and tasks are kept without the link. This cannot be undone.',
    confirmLabel: 'Delete',
    danger: true,
  })
  if (!ok) return
  try {
    await api.del(`/contacts/${props.id}`)
    toast.success(`${label('contact')} deleted.`)
    await qc.invalidateQueries({ queryKey: ['contacts'] })
    await router.push('/contacts')
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
</script>

<template>
  <div>
    <StateViews v-if="q.isLoading.value" kind="loading" />
    <div v-else-if="notFound" class="card">
      <StateViews kind="empty" :title="`${label('contact')} not found`" message="It may have been deleted or merged into another record.">
        <RouterLink to="/contacts" class="text-sm text-brand-700 hover:underline">Back to {{ label('contact', true).toLowerCase() }}</RouterLink>
      </StateViews>
    </div>
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <template v-else-if="c">
      <PageHeader :title="name" :subtitle="[c.job_title, c.company_name].filter(Boolean).join(' at ') || undefined" back="/contacts">
        <template #meta>
          <div class="mt-1 flex flex-wrap items-center gap-2">
            <UiBadge :value="c.status" />
            <UiBadge v-if="c.is_decision_maker" tone="violet">decision maker</UiBadge>
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
          <section v-if="c.notes_summary" class="card p-4" aria-labelledby="contact-summary">
            <h2 id="contact-summary" class="mb-2 text-sm font-semibold text-slate-800">Summary</h2>
            <p class="text-sm whitespace-pre-line text-slate-700">{{ c.notes_summary }}</p>
          </section>
          <section v-if="defs.length" class="card p-4" aria-labelledby="contact-cf">
            <h2 id="contact-cf" class="mb-3 text-sm font-semibold text-slate-800">Custom fields</h2>
            <CustomFieldsEditor :model-value="customFields(c.custom_fields)" entity-type="contact" readonly />
          </section>
        </div>
        <div class="min-w-0 space-y-4">
          <section class="card p-4" aria-labelledby="contact-links">
            <h2 id="contact-links" class="mb-2 text-sm font-semibold text-slate-800">Works with</h2>
            <ul class="space-y-2 text-sm">
              <li class="flex items-center gap-2">
                <Building2 class="h-4 w-4 shrink-0 text-slate-400" aria-hidden="true" />
                <span class="text-slate-500">{{ label('company') }}:</span>
                <RouterLink v-if="c.company_id" :to="`/companies/${c.company_id}`" class="truncate text-brand-700 hover:underline" data-testid="contact-company">{{ c.company_name ?? 'Open' }}</RouterLink>
                <span v-else class="text-slate-400">None</span>
              </li>
              <li class="flex items-center gap-2">
                <Truck class="h-4 w-4 shrink-0 text-slate-400" aria-hidden="true" />
                <span class="text-slate-500">{{ label('supplier') }}:</span>
                <RouterLink v-if="c.supplier_id" :to="`/suppliers/${c.supplier_id}`" class="truncate text-brand-700 hover:underline">{{ c.supplier_name ?? 'Open' }}</RouterLink>
                <span v-else class="text-slate-400">None</span>
              </li>
            </ul>
            <button v-if="auth.can('crm.write')" type="button" class="mt-3 text-xs text-brand-700 hover:underline" @click="editing = true">Change links</button>
          </section>
          <section class="card p-4" aria-labelledby="contact-tags">
            <h2 id="contact-tags" class="mb-2 text-sm font-semibold text-slate-800">Tags</h2>
            <TagEditor entity-type="contact" :entity-id="c.id" :tags="c.tags" :can-edit="auth.can('crm.write')" @updated="setTags" />
          </section>
        </div>
      </div>

      <div class="mt-6"><RecordPanels entity-type="contact" :entity-id="c.id" /></div>

      <ContactFormModal :open="editing" :contact="c" @close="editing = false" @saved="saved" />
      <MergeDialog :open="merging" entity-type="contact" :record="c" :record-label="name" @close="merging = false" @merged="merged" />
    </template>
  </div>
</template>
