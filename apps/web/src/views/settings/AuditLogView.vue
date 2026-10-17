<script setup lang="ts">
// The organization's audit log, newest first, with filters and "load older" pagination.
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { refDebounced } from '@vueuse/core'
import { useInfiniteQuery } from '@tanstack/vue-query'
import { api, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import StateViews from '@/components/ui/StateViews.vue'
import UiButton from '@/components/ui/UiButton.vue'
import EntityPicker from '@/components/record/EntityPicker.vue'
import ChangesView from '@/components/record/ChangesView.vue'
import SettingsNav from '@/components/settings/SettingsNav.vue'
import { entityLabel, entityLink } from '@/lib/entities'
import { formatDateTime, relativeTime, titleCase } from '@/lib/format'
import { AUDIT_ACTION_PREFIXES, AUDIT_ENTITY_LABELS, AUDIT_ENTITY_TYPES } from '@/components/settings/options'

const PAGE = 50
const route = useRoute()
const router = useRouter()

const str = (k: string) => (typeof route.query[k] === 'string' ? (route.query[k] as string) : '')
const entityType = computed({ get: () => str('entity_type'), set: (v) => setQuery('entity_type', v) })
const actor = computed<string | null>({ get: () => str('actor_user_id') || null, set: (v) => setQuery('actor_user_id', v ?? '') })
const actionText = ref(str('action'))
const actionDebounced = refDebounced(actionText, 300)
watch(actionDebounced, (v) => setQuery('action', v.trim()))
watch(
  () => route.query.action,
  (v) => {
    if ((typeof v === 'string' ? v : '') !== actionText.value.trim()) actionText.value = typeof v === 'string' ? v : ''
  },
)

function setQuery(k: string, v: string) {
  const next: Record<string, string> = { ...(route.query as Record<string, string>), [k]: v }
  for (const key of Object.keys(next)) if (!next[key]) delete next[key]
  void router.replace({ query: next })
}
function clearFilters() {
  actionText.value = ''
  void router.replace({ query: {} })
}
const filtered = computed(() => !!(entityType.value || actor.value || str('action')))

const q = useInfiniteQuery({
  queryKey: computed(() => ['audit-logs', { entity_type: entityType.value, actor_user_id: actor.value, action: str('action') }]),
  queryFn: ({ pageParam, signal }) =>
    api.get<Schemas['AuditPageDoc']>(
      '/audit-logs',
      { entity_type: entityType.value, actor_user_id: actor.value ?? undefined, action: str('action'), limit: PAGE, before_id: pageParam ?? undefined },
      signal,
    ),
  initialPageParam: null as number | null,
  getNextPageParam: (last) => last.next_before_id ?? null,
})
const entries = computed(() => (q.data.value?.pages ?? []).flatMap((p) => p.items))

const IP_HINT = 'Address recorded by the server for the request'
const typeName = (t: string) => AUDIT_ENTITY_LABELS[t] ?? titleCase(entityLabel(t))
</script>

<template>
  <div>
    <SettingsNav />
    <PageHeader title="Audit log" subtitle="Who changed what, and when. Entries cannot be edited or deleted from REA." />

    <form class="card mb-4 grid grid-cols-1 gap-3 p-4 sm:grid-cols-2 lg:grid-cols-[12rem_1fr_14rem_auto] lg:items-end" role="search" @submit.prevent>
      <div>
        <label for="audit-entity" class="label">Record type</label>
        <select id="audit-entity" v-model="entityType" class="input">
          <option value="">All types</option>
          <option v-for="t in AUDIT_ENTITY_TYPES" :key="t" :value="t">{{ typeName(t) }}</option>
        </select>
      </div>
      <div>
        <label for="audit-actor" class="label">Performed by</label>
        <EntityPicker v-model="actor" type="user" input-id="audit-actor" placeholder="Anyone" />
      </div>
      <div>
        <label for="audit-action" class="label">Action starts with</label>
        <input id="audit-action" v-model="actionText" class="input font-mono" list="audit-actions" placeholder="e.g. user." spellcheck="false" />
        <datalist id="audit-actions">
          <option v-for="a in AUDIT_ACTION_PREFIXES" :key="a" :value="a" />
        </datalist>
      </div>
      <UiButton :disabled="!filtered && !actionText" @click="clearFilters">Clear filters</UiButton>
    </form>

    <StateViews v-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <StateViews v-else-if="entries.length === 0" kind="empty" :title="filtered ? 'No matching entries' : 'No audit entries yet'" />
    <template v-else>
      <ol class="card divide-y divide-slate-100" aria-label="Audit entries">
        <li v-for="e in entries" :key="e.id" class="grid grid-cols-1 gap-1 px-4 py-3 sm:grid-cols-[11rem_1fr]" :data-testid="`audit-${e.id}`">
          <p class="text-xs text-slate-500">
            <time :datetime="e.occurred_at" :title="formatDateTime(e.occurred_at)">{{ formatDateTime(e.occurred_at) }}</time>
            <span class="block">{{ relativeTime(e.occurred_at) }}</span>
          </p>
          <div class="min-w-0">
            <p class="text-sm text-slate-800">
              <span class="font-medium">{{ e.actor_label || 'System' }}</span>
              <span v-if="e.actor_device_id" class="text-xs text-slate-500"> (via browser extension)</span>
              · {{ e.summary || e.action }}
            </p>
            <p class="mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-slate-500">
              <code class="rounded bg-slate-100 px-1 text-[11px] text-slate-700">{{ e.action }}</code>
              <template v-if="e.entity_type">
                <RouterLink v-if="entityLink(e.entity_type, e.entity_id)" :to="entityLink(e.entity_type, e.entity_id)!" class="text-brand-700 hover:underline">{{ typeName(e.entity_type) }}</RouterLink>
                <span v-else>{{ typeName(e.entity_type) }}</span>
              </template>
              <span v-if="e.ip_address" :title="IP_HINT">{{ e.ip_address }}</span>
            </p>
            <ChangesView v-if="e.changes" :changes="e.changes" class="mt-1" />
          </div>
        </li>
      </ol>
      <div class="mt-4 flex justify-center">
        <UiButton v-if="q.hasNextPage.value" :loading="q.isFetchingNextPage.value" @click="q.fetchNextPage()">Load older entries</UiButton>
        <p v-else class="text-xs text-slate-500">You have reached the oldest matching entry.</p>
      </div>
    </template>
  </div>
</template>
