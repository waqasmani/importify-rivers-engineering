<script setup lang="ts">
// "Find duplicates" for one record: lists candidates from GET /<type>/duplicates and merges the
// selected ones into this record with POST /<type>/{id}/merge after an explicit confirmation.
import { computed, ref, watch } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { api, errorMessage, type Schemas } from '@/api/client'
import UiModal from '@/components/ui/UiModal.vue'
import UiButton from '@/components/ui/UiButton.vue'
import StateViews from '@/components/ui/StateViews.vue'
import { useConfirm } from '@/composables/useConfirm'
import { useToast } from '@/composables/useToast'
import { useSettings } from '@/stores/settings'
import { entityLink } from '@/lib/entities'
import { percent } from '@/lib/format'
import { CRM_PATH, type MergeEntity } from './crm'

type Candidate = Schemas['DuplicateCandidate']
interface Target {
  id: string
  name?: string
  domain?: string | null
  website?: string | null
  first_name?: string
  last_name?: string
  email?: string | null
  company_id?: string | null
}

const props = defineProps<{ open: boolean; entityType: MergeEntity; record: Target; recordLabel: string }>()
const emit = defineEmits<{ close: []; merged: [record: unknown] }>()
const { confirm } = useConfirm()
const toast = useToast()
const { label } = useSettings()
const selected = ref<string[]>([])
const merging = ref(false)
const error = ref('')

const params = computed<Record<string, string>>(() => {
  const r = props.record
  const p: Record<string, string> = { exclude_id: r.id }
  if (props.entityType === 'contact') {
    if (r.first_name) p.first_name = r.first_name
    if (r.last_name) p.last_name = r.last_name
    if (r.email) p.email = r.email
    if (r.company_id) p.company_id = r.company_id
  } else {
    if (r.name) p.name = r.name
    if (r.domain) p.domain = r.domain
    else if (r.website) p.website = r.website
  }
  return p
})

const q = useQuery({
  queryKey: computed(() => ['duplicates', props.entityType, params.value]),
  queryFn: ({ signal }) => api.get<Schemas['DuplicateCandidateItems']>(`${CRM_PATH[props.entityType]}/duplicates`, params.value, signal),
  enabled: computed(() => props.open),
  staleTime: 0,
})
const items = computed(() => q.data.value?.items ?? [])

watch(
  () => props.open,
  (o) => {
    if (o) {
      selected.value = []
      error.value = ''
      void q.refetch()
    }
  },
)

function detail(c: Candidate): string {
  const r = (c.record ?? {}) as Record<string, unknown>
  const parts = [r.domain, r.email, r.country, r.job_title].filter((x): x is string => typeof x === 'string' && x !== '')
  return parts.join(' · ')
}

async function merge() {
  const chosen = items.value.filter((c) => selected.value.includes(c.id))
  if (!chosen.length) return
  const plural = label(props.entityType, true).toLowerCase()
  const ok = await confirm({
    title: `Merge ${chosen.length} ${chosen.length === 1 ? label(props.entityType).toLowerCase() : plural} into “${props.recordLabel}”?`,
    message: `These records will be deleted after their notes, tasks, files, tags and linked records are moved to “${props.recordLabel}”. Empty fields are filled from them. This cannot be undone.`,
    details: chosen.map((c) => `${c.label} (${percent(c.score)} match)`),
    confirmLabel: 'Merge records',
    danger: true,
  })
  if (!ok) return
  merging.value = true
  error.value = ''
  try {
    const res = await api.post<unknown>(`${CRM_PATH[props.entityType]}/${props.record.id}/merge`, { source_ids: chosen.map((c) => c.id) } satisfies Schemas['MergeInput'])
    toast.success(`Merged ${chosen.length} into ${props.recordLabel}.`)
    emit('merged', res)
    emit('close')
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    merging.value = false
  }
}
</script>

<template>
  <UiModal :open="open" :title="`Possible duplicates of “${recordLabel}”`" size="lg" @close="emit('close')">
    <p class="mb-3 text-sm text-slate-600">Matches are based on the name, domain or e-mail of this record. Select the records to merge into this one.</p>
    <p v-if="error" class="mb-3 rounded-md bg-red-50 px-3 py-2 text-sm text-red-700" role="alert">{{ error }}</p>
    <StateViews v-if="q.isFetching.value && !q.data.value" kind="loading" message="Looking for duplicates…" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <StateViews v-else-if="items.length === 0" kind="empty" title="No likely duplicates found" />
    <ul v-else class="divide-y divide-slate-100 rounded-md border border-slate-200" data-testid="duplicate-candidates">
      <li v-for="c in items" :key="c.id" class="flex items-start gap-3 px-3 py-2.5">
        <input :id="`dup-${c.id}`" v-model="selected" type="checkbox" :value="c.id" class="mt-1 rounded border-slate-300" />
        <label :for="`dup-${c.id}`" class="min-w-0 flex-1 cursor-pointer">
          <span class="block text-sm font-medium text-slate-900">{{ c.label }}</span>
          <span v-if="detail(c)" class="block truncate text-xs text-slate-500">{{ detail(c) }}</span>
          <span class="mt-0.5 block text-xs text-slate-600">{{ percent(c.score) }} match<template v-if="(c.reasons ?? []).length"> · {{ (c.reasons ?? []).join(', ') }}</template></span>
        </label>
        <RouterLink v-if="entityLink(entityType, c.id)" :to="entityLink(entityType, c.id)!" class="shrink-0 text-xs text-brand-700 hover:underline" target="_blank">Open</RouterLink>
      </li>
    </ul>
    <template #footer>
      <UiButton variant="ghost" @click="emit('close')">Close</UiButton>
      <UiButton variant="danger" :disabled="selected.length === 0" :loading="merging" data-testid="merge-selected" @click="merge">Merge {{ selected.length || '' }} into this record</UiButton>
    </template>
  </UiModal>
</template>
