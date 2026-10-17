<script setup lang="ts">
// Finds likely duplicates among a page of records (GET /<type>/duplicates for each record) and
// merges a group into a chosen primary record (POST /<type>/{primary}/merge).
import { computed, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { api, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import StateViews from '@/components/ui/StateViews.vue'
import UiTabs from '@/components/ui/UiTabs.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiPagination from '@/components/ui/UiPagination.vue'
import { CRM_PATH, type MergeEntity } from '@/components/crm/crm'
import { duplicateParams, groupDuplicates, type DuplicateGroup } from '@/components/crm/duplicates'
import type { PageOf } from '@/composables/useListQuery'
import { useAuth } from '@/stores/auth'
import { useSettings } from '@/stores/settings'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { entityLink } from '@/lib/entities'
import { formatDate, percent } from '@/lib/format'

type Rec = Record<string, unknown> & { id: string }
const TYPES: MergeEntity[] = ['company', 'contact', 'supplier']
const PAGE = 25

const route = useRoute()
const router = useRouter()
const auth = useAuth()
const qc = useQueryClient()
const toast = useToast()
const { confirm } = useConfirm()
const { label } = useSettings()
const canMerge = computed(() => auth.can('crm.delete'))

const type = computed<MergeEntity>(() => {
  const t = route.query.type
  return typeof t === 'string' && (TYPES as string[]).includes(t) ? (t as MergeEntity) : 'company'
})
const offset = computed(() => Math.max(0, Number(route.query.offset ?? 0) || 0))
const tab = computed({
  get: () => type.value,
  set: (t: string) => void router.replace({ query: { type: t } }),
})
const tabs = computed(() => TYPES.map((t) => ({ key: t, label: label(t, true) })))

const progress = ref({ done: 0, total: 0 })
const scan = useQuery({
  queryKey: computed(() => ['duplicate-scan', type.value, offset.value]),
  queryFn: async ({ signal }) => {
    const path = CRM_PATH[type.value]
    const page = await api.get<PageOf<Rec>>(path, { limit: PAGE, offset: offset.value, sort: '-created' }, signal)
    progress.value = { done: 0, total: page.items.length }
    const scanned: { record: Rec; candidates: Schemas['DuplicateCandidate'][] }[] = []
    const queue = [...page.items]
    // A few requests at a time keeps the scan quick without flooding the API.
    const worker = async () => {
      for (let r = queue.shift(); r; r = queue.shift()) {
        const params = duplicateParams(type.value, r)
        const res = params ? await api.get<Schemas['DuplicateCandidateItems']>(`${path}/duplicates`, params, signal) : { items: [] }
        scanned.push({ record: r, candidates: res.items })
        progress.value = { ...progress.value, done: progress.value.done + 1 }
      }
    }
    await Promise.all([worker(), worker(), worker(), worker()])
    return { total: page.total, count: page.items.length, groups: groupDuplicates(type.value, scanned) }
  },
})

// Dismissals only hide a group in this browser tab; the API has no endpoint to store them.
const dismissed = ref(new Set<string>())
const groups = computed(() => (scan.data.value?.groups ?? []).filter((g) => !dismissed.value.has(g.key)))
const choice = reactive<Record<string, { primary: string; merge: string[] }>>({})
watch(
  () => scan.data.value?.groups,
  (gs) => {
    for (const g of gs ?? []) {
      if (!choice[g.key]) choice[g.key] = { primary: g.members[0]!.id, merge: g.members.slice(1).map((m) => m.id) }
    }
  },
  { immediate: true },
)
function setPrimary(g: DuplicateGroup, id: string) {
  const c = choice[g.key]!
  const prev = c.primary
  c.primary = id
  c.merge = c.merge.filter((x) => x !== id)
  if (!c.merge.includes(prev)) c.merge.push(prev)
}
function nameOf(g: DuplicateGroup, id: string) {
  return g.members.find((m) => m.id === id)?.label ?? id
}

const merging = ref<string | null>(null)
async function merge(g: DuplicateGroup) {
  const c = choice[g.key]!
  const sources = c.merge.filter((id) => id !== c.primary)
  if (!sources.length) return
  const keep = nameOf(g, c.primary)
  const ok = await confirm({
    title: `Merge ${sources.length} into “${keep}”?`,
    message: `These ${label(type.value, true).toLowerCase()} are deleted after their notes, tasks, files, tags and linked records move to “${keep}”. Empty fields of “${keep}” are filled from them. This cannot be undone.`,
    details: sources.map((id) => nameOf(g, id)),
    confirmLabel: 'Merge records',
    danger: true,
  })
  if (!ok) return
  merging.value = g.key
  try {
    await api.post(`${CRM_PATH[type.value]}/${c.primary}/merge`, { source_ids: sources } satisfies Schemas['MergeInput'])
    toast.success(`Merged ${sources.length} into ${keep}.`)
    await Promise.all([qc.invalidateQueries({ queryKey: ['duplicate-scan'] }), qc.invalidateQueries({ queryKey: [type.value === 'company' ? 'companies' : `${type.value}s`] })])
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    merging.value = null
  }
}
function dismiss(g: DuplicateGroup) {
  dismissed.value = new Set([...dismissed.value, g.key])
}
function setOffset(o: number) {
  void router.replace({ query: { type: type.value, ...(o ? { offset: String(o) } : {}) } })
}
</script>

<template>
  <div>
    <PageHeader title="Duplicates" subtitle="Find records that look like the same organization or person, then merge them." />
    <UiTabs v-model="tab" :tabs="tabs">
      <p class="mb-3 text-sm text-slate-600">
        Checks the {{ PAGE }} most recently created {{ label(type, true).toLowerCase() }} on this page against all others by name, domain or e-mail.
        Dismiss only hides a group until you reload this page; REA does not store dismissals.
        <span v-if="!canMerge" class="font-medium">Merging requires delete permission.</span>
      </p>
      <div v-if="scan.isFetching.value" class="card">
        <StateViews kind="loading" :message="progress.total ? `Checked ${progress.done} of ${progress.total} records…` : 'Loading records…'" />
      </div>
      <StateViews v-else-if="scan.error.value" kind="error" :message="errorMessage(scan.error.value)" @retry="scan.refetch()" />
      <template v-else-if="scan.data.value">
        <div v-if="groups.length === 0" class="card">
          <StateViews kind="empty" :title="`No likely duplicates among these ${label(type, true).toLowerCase()}`" :message="scan.data.value.total > PAGE ? 'Check the next page of records.' : undefined" />
        </div>
        <ul v-else class="space-y-3" data-testid="duplicate-groups">
          <li v-for="g in groups" :key="g.key" class="card p-4">
            <div class="mb-2 flex flex-wrap items-center justify-between gap-2">
              <h2 class="text-sm font-semibold text-slate-800">{{ g.members.length }} similar {{ label(type, true).toLowerCase() }} · best match {{ percent(g.score) }}</h2>
              <div class="flex gap-2">
                <UiButton size="sm" variant="ghost" @click="dismiss(g)">Dismiss</UiButton>
                <UiButton v-if="canMerge" size="sm" variant="danger" :loading="merging === g.key" :disabled="!choice[g.key]?.merge.length" @click="merge(g)">Merge into primary</UiButton>
              </div>
            </div>
            <fieldset>
              <legend class="sr-only">Choose the record to keep</legend>
              <ul class="divide-y divide-slate-100 rounded-md border border-slate-200">
                <li v-for="m in g.members" :key="m.id" class="flex flex-wrap items-center gap-3 px-3 py-2">
                  <label class="flex items-center gap-1.5 text-xs text-slate-600">
                    <input type="radio" :name="`primary-${g.key}`" :checked="choice[g.key]?.primary === m.id" :disabled="!canMerge" @change="setPrimary(g, m.id)" /> Keep
                  </label>
                  <label v-if="canMerge && choice[g.key]" class="flex items-center gap-1.5 text-xs text-slate-600">
                    <input v-model="choice[g.key]!.merge" type="checkbox" :value="m.id" :disabled="choice[g.key]?.primary === m.id" class="rounded border-slate-300" /> Merge
                  </label>
                  <div class="min-w-0 flex-1">
                    <RouterLink :to="entityLink(type, m.id) ?? '/'" class="block truncate text-sm font-medium text-brand-700 hover:underline">{{ m.label }}</RouterLink>
                    <span class="block truncate text-xs text-slate-500">{{ [m.detail, m.createdAt ? `created ${formatDate(m.createdAt)}` : ''].filter(Boolean).join(' · ') }}</span>
                  </div>
                  <span v-if="choice[g.key]?.primary === m.id" class="text-xs font-medium text-green-700">Primary</span>
                </li>
              </ul>
            </fieldset>
            <ul class="mt-2 space-y-0.5 text-xs text-slate-600">
              <li v-for="e in g.evidence" :key="`${e.a}-${e.b}`">{{ nameOf(g, e.a) }} ↔ {{ nameOf(g, e.b) }}: {{ percent(e.score) }}<template v-if="e.reasons.length"> · {{ e.reasons.join(', ') }}</template></li>
            </ul>
          </li>
        </ul>
        <p v-if="dismissed.size" class="mt-3 text-xs text-slate-500">{{ dismissed.size }} dismissed. Dismissed groups are only hidden until you reload this page; REA does not store dismissals.</p>
        <UiPagination :total="scan.data.value.total" :limit="PAGE" :offset="offset" @update:offset="setOffset" />
      </template>
    </UiTabs>
  </div>
</template>
