<script setup lang="ts">
// Compact list of records related to the current one: the first page of a list endpoint with filters.
import { computed } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { api, errorMessage } from '@/api/client'
import StateViews from '@/components/ui/StateViews.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import { entityLink } from '@/lib/entities'
import { formatMoney } from '@/lib/format'
import type { PageOf } from '@/composables/useListQuery'
import { contactName, type CrmEntity } from './crm'

type Row = Record<string, unknown> & { id: string }

const props = withDefaults(
  defineProps<{
    title: string
    type: CrmEntity
    endpoint: string
    query?: Record<string, string>
    /** Link to the full, filtered list page. */
    viewAll?: string
    limit?: number
  }>(),
  { query: () => ({}), viewAll: undefined, limit: 10 },
)

const q = useQuery({
  queryKey: computed(() => ['related', props.endpoint, props.query]),
  queryFn: ({ signal }) => api.get<PageOf<Row>>(props.endpoint, { ...props.query, limit: props.limit }, signal),
})

const rows = computed(() => q.data.value?.items ?? [])
const total = computed(() => q.data.value?.total ?? 0)

function s(r: Row, k: string): string {
  const v = r[k]
  return typeof v === 'string' ? v : typeof v === 'number' ? String(v) : ''
}
function titleOf(r: Row): string {
  if (props.type === 'contact') return contactName(r as { full_name?: string | null; first_name?: string; last_name?: string; email?: string | null })
  return s(r, 'name') || s(r, 'title') || 'Untitled'
}
function subOf(r: Row): string {
  switch (props.type) {
    case 'contact':
      return [s(r, 'job_title'), s(r, 'email')].filter(Boolean).join(' · ')
    case 'lead':
      return [s(r, 'stage_name'), r.estimated_value ? formatMoney(s(r, 'estimated_value'), s(r, 'currency')) : ''].filter(Boolean).join(' · ')
    case 'opportunity':
      return [s(r, 'stage_name'), r.amount ? formatMoney(s(r, 'amount'), s(r, 'currency')) : ''].filter(Boolean).join(' · ')
    case 'supplier':
      return [s(r, 'supplier_type').replace(/_/g, ' '), s(r, 'country')].filter(Boolean).join(' · ')
    default:
      return [s(r, 'domain'), s(r, 'country')].filter(Boolean).join(' · ')
  }
}
function badgeOf(r: Row): string {
  if (props.type === 'opportunity') return s(r, 'status')
  if (props.type === 'contact' && r.is_decision_maker) return 'decision maker'
  return ''
}
</script>

<template>
  <section class="card p-4" :aria-label="title">
    <div class="mb-2 flex items-center justify-between gap-2">
      <h2 class="text-sm font-semibold text-slate-800">{{ title }} <span v-if="q.data.value" class="font-normal text-slate-500">({{ total }})</span></h2>
      <div class="flex items-center gap-2">
        <slot name="actions" />
        <RouterLink v-if="viewAll && total > rows.length" :to="viewAll" class="text-xs text-brand-700 hover:underline">View all</RouterLink>
      </div>
    </div>
    <StateViews v-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <p v-else-if="rows.length === 0" class="py-3 text-sm text-slate-500">None yet.</p>
    <ul v-else class="divide-y divide-slate-100">
      <li v-for="r in rows" :key="r.id" class="flex items-center gap-2 py-2">
        <div class="min-w-0 flex-1">
          <RouterLink :to="entityLink(type, r.id) ?? '/'" class="block truncate text-sm font-medium text-slate-800 hover:underline">{{ titleOf(r) }}</RouterLink>
          <p v-if="subOf(r)" class="truncate text-xs text-slate-500">{{ subOf(r) }}</p>
        </div>
        <UiBadge v-if="badgeOf(r)" :value="badgeOf(r)" />
      </li>
    </ul>
  </section>
</template>
