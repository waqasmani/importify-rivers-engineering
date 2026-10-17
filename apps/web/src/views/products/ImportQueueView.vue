<script setup lang="ts">
// Product import queue: everything submitted from the extension, CSV, analysis or by hand, with the
// imports REA parked as possible duplicates highlighted for a decision.
import { computed } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { AlertTriangle, Plus } from 'lucide-vue-next'
import { api, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import DataTable, { type Column } from '@/components/ui/DataTable.vue'
import UiPagination from '@/components/ui/UiPagination.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import { useListQuery, type PageOf } from '@/composables/useListQuery'
import { useAuth } from '@/stores/auth'
import { useDeviceNames } from '@/components/products/useDeviceNames'
import { asRecord, dupSummaries, needsDecision, safeHref, str, IMPORT_SOURCES, IMPORT_STATUSES, IMPORT_STATUS_LABELS } from '@/components/products/product-utils'
import { formatDateTime, relativeTime, titleCase } from '@/lib/format'

type Item = Schemas['ImportListItem']

const auth = useAuth()
const { deviceName } = useDeviceNames()
const list = useListQuery<Item>('imports', '/imports', ['status', 'source', 'created_by'])

const columns: Column[] = [
  { key: 'title', label: 'Product' },
  { key: 'status', label: 'Status' },
  { key: 'source', label: 'Source', class: 'hidden md:table-cell' },
  { key: 'duplicates', label: 'Duplicates', class: 'hidden sm:table-cell' },
  { key: 'created_at', label: 'Created' },
  { key: 'device', label: 'Device / user', class: 'hidden lg:table-cell' },
]

// The list endpoint omits the raw payload; the normalized copy carries the title and URL.
function info(row: Item) {
  const n = asRecord(row.normalized)
  const p = asRecord(row.payload)
  return {
    title: str(n.title) || str(p.title) || row.product_title || 'Untitled product',
    url: str(n.source_url) || str(p.url),
    domain: str(n.source_domain),
  }
}

const pendingCount = useQuery({
  queryKey: ['imports', 'needs-decision-count'],
  queryFn: async ({ signal }) => {
    const [dup, pending] = await Promise.all([
      api.get<PageOf<Item>>('/imports', { status: 'duplicate', limit: 1 }, signal),
      api.get<PageOf<Item>>('/imports', { status: 'pending', limit: 1 }, signal),
    ])
    return dup.total + pending.total
  },
  refetchInterval: 60_000,
})
const waiting = computed(() => pendingCount.data.value ?? 0)
const filtered = computed(() => !!(list.filters.status || list.filters.source || list.filters.created_by))
</script>

<template>
  <div>
    <PageHeader title="Import queue" subtitle="Products submitted from the browser extension, CSV files, analyses and manual imports. Imports never publish anything; they create or update draft products in REA.">
      <template #actions>
        <RouterLink v-if="auth.can('products.write')" to="/imports/new" class="inline-flex items-center gap-1.5 rounded-md bg-brand-600 whitespace-nowrap px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700">
          <Plus class="h-4 w-4" aria-hidden="true" /> New import
        </RouterLink>
      </template>
    </PageHeader>

    <div v-if="waiting > 0" class="mb-4 flex flex-wrap items-center gap-2 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900" role="status" data-testid="needs-decision-banner">
      <AlertTriangle class="h-4 w-4 shrink-0" aria-hidden="true" />
      <span>{{ waiting }} import{{ waiting === 1 ? '' : 's' }} need{{ waiting === 1 ? 's' : '' }} a decision.</span>
      <button v-if="list.filters.status !== 'duplicate'" type="button" class="font-medium underline hover:no-underline" @click="list.set('status', 'duplicate')">Show possible duplicates</button>
    </div>

    <form class="mb-4 flex flex-wrap items-end gap-3" role="search" aria-label="Filter imports" @submit.prevent>
      <div class="w-full sm:w-48">
        <label for="imp-status" class="label">Status</label>
        <select id="imp-status" class="input" :value="list.filters.status" @change="list.set('status', ($event.target as HTMLSelectElement).value)">
          <option value="">All statuses</option>
          <option v-for="s in IMPORT_STATUSES" :key="s" :value="s">{{ IMPORT_STATUS_LABELS[s] ?? titleCase(s) }}</option>
        </select>
      </div>
      <div class="w-full sm:w-44">
        <label for="imp-source" class="label">Source</label>
        <select id="imp-source" class="input" :value="list.filters.source" @change="list.set('source', ($event.target as HTMLSelectElement).value)">
          <option value="">All sources</option>
          <option v-for="s in IMPORT_SOURCES" :key="s" :value="s">{{ s === 'csv' ? 'CSV' : s === 'api' ? 'API' : titleCase(s) }}</option>
        </select>
      </div>
      <label class="flex items-center gap-2 pb-2 text-sm text-slate-700">
        <input type="checkbox" class="rounded border-slate-300" :checked="list.filters.created_by === 'me'" @change="list.set('created_by', ($event.target as HTMLInputElement).checked ? 'me' : '')" />
        Only my imports
      </label>
      <button v-if="filtered" type="button" class="pb-2 text-sm text-brand-700 hover:underline" @click="$router.replace({ query: {} })">Clear filters</button>
    </form>

    <DataTable
      :columns="columns"
      :rows="list.items.value"
      :loading="list.loading.value"
      :error="list.error.value"
      :empty-title="filtered ? 'No imports match these filters' : 'No imports yet'"
      empty-message="Capture a product with the browser extension, import a CSV file on the products page, or start a manual import."
      @retry="list.refetch()"
    >
      <template #cell-title="{ row }">
        <div class="flex min-w-0 max-w-md items-start gap-2" :class="needsDecision(row.status) ? 'border-l-4 border-amber-400 pl-2' : ''">
          <div class="min-w-0">
            <RouterLink :to="`/imports/${row.id}`" class="block truncate font-medium text-brand-700 hover:underline">{{ info(row).title }}</RouterLink>
            <a v-if="safeHref(info(row).url)" :href="safeHref(info(row).url)" target="_blank" rel="noopener noreferrer nofollow" class="block truncate text-xs text-slate-500 hover:underline">{{ info(row).url }}</a>
            <span v-else-if="row.product_title" class="block truncate text-xs text-slate-500">Product: {{ row.product_title }}</span>
          </div>
        </div>
      </template>
      <template #cell-status="{ row }">
        <div class="flex flex-col items-start gap-1">
          <UiBadge :value="row.status">{{ IMPORT_STATUS_LABELS[row.status] ?? titleCase(row.status) }}</UiBadge>
          <span v-if="needsDecision(row.status)" class="text-xs font-medium text-amber-700">Needs decision</span>
          <span v-if="row.error" class="max-w-48 truncate text-xs text-red-600" :title="row.error">{{ row.error }}</span>
        </div>
      </template>
      <template #cell-source="{ row }">{{ row.source === 'csv' ? 'CSV' : row.source === 'api' ? 'API' : titleCase(row.source) }}</template>
      <template #cell-duplicates="{ row }">
        <span :class="dupSummaries(row.duplicate_candidates).length ? 'font-medium text-amber-700' : 'text-slate-400'">{{ dupSummaries(row.duplicate_candidates).length }}</span>
      </template>
      <template #cell-created_at="{ row }">
        <time :datetime="row.created_at" :title="formatDateTime(row.created_at)" class="whitespace-nowrap">{{ relativeTime(row.created_at) }}</time>
      </template>
      <template #cell-device="{ row }">
        <span class="block">{{ deviceName(row.device_id) || 'Web or API' }}</span>
        <span v-if="row.created_by_name" class="block text-xs text-slate-500">{{ row.created_by_name }}</span>
      </template>
    </DataTable>
    <UiPagination :total="list.total.value" :limit="list.limit.value" :offset="list.offset.value" @update:offset="(o) => list.set('offset', String(o))" />
  </div>
</template>
