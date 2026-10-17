<script setup lang="ts">
// Customer orders received from your own systems or entered by hand, newest first. Filters live in
// the URL. Supplier orders are prepared automatically and handed off only after confirmation.
import { computed, ref, watch } from 'vue'
import { refDebounced } from '@vueuse/core'
import { CircleAlert, Plus } from 'lucide-vue-next'
import type { Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import DataTable, { type Column } from '@/components/ui/DataTable.vue'
import UiPagination from '@/components/ui/UiPagination.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import { useListQuery } from '@/composables/useListQuery'
import { useAuth } from '@/stores/auth'
import { formatDateTime, formatMoney, relativeTime } from '@/lib/format'
import { ORDER_STATUSES, ORDER_STATUS_LABELS, dayAfter, dayStart, orderTone, regionName } from '@/components/orders/order-utils'

type Row = Schemas['OrderListItem']
const FILTERS = ['q', 'status', 'needs_attention', 'channel', 'from', 'to']

const auth = useAuth()
const list = useListQuery<Row>('orders', '/orders', FILTERS)

const search = ref(list.filters.q ?? '')
const debounced = refDebounced(search, 300)
watch(debounced, (v) => {
  if (v.trim() !== (list.filters.q ?? '')) list.set('q', v.trim())
})
watch(
  () => list.filters.q,
  (v) => {
    if ((v ?? '') !== search.value.trim()) search.value = v ?? ''
  },
)

/** The local calendar day of an RFC 3339 bound kept in the URL (the upper bound is exclusive). */
function localDay(iso: string | undefined, exclusive = false): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  if (exclusive) d.setDate(d.getDate() - 1)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}
const fromDay = computed(() => localDay(list.filters.from))
const toDay = computed(() => localDay(list.filters.to, true))

const columns: Column[] = [
  { key: 'number', label: 'Order' },
  { key: 'customer', label: 'Customer', class: 'hidden sm:table-cell' },
  { key: 'status', label: 'Status' },
  { key: 'lines', label: 'Lines', class: 'hidden md:table-cell' },
  { key: 'total', label: 'Total', class: 'hidden md:table-cell text-right' },
  { key: 'source', label: 'Source', class: 'hidden lg:table-cell' },
  { key: 'ordered', label: 'Ordered', class: 'hidden lg:table-cell' },
]
const filtered = computed(() => FILTERS.some((k) => !!list.filters[k]))
</script>

<template>
  <div>
    <PageHeader title="Orders" subtitle="Customer orders from your storefront, ERP or scripts. REA prepares the supplier orders; each is handed off only after a person confirms it.">
      <template #actions>
        <RouterLink v-if="auth.can('orders.write')" to="/orders/new" class="inline-flex shrink-0 items-center gap-1.5 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium whitespace-nowrap text-white shadow-xs hover:bg-brand-700">
          <Plus class="h-4 w-4" aria-hidden="true" /> New order
        </RouterLink>
      </template>
    </PageHeader>

    <form class="mb-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-6" role="search" aria-label="Filter orders" @submit.prevent="list.set('q', search.trim())">
      <div class="sm:col-span-2">
        <label for="o-q" class="label">Search</label>
        <input id="o-q" v-model="search" type="search" class="input" placeholder="Order number or external order ID" />
      </div>
      <div>
        <label for="o-status" class="label">Status</label>
        <select id="o-status" class="input" :value="list.filters.status" @change="list.set('status', ($event.target as HTMLSelectElement).value)">
          <option value="">Any</option>
          <option v-for="s in ORDER_STATUSES" :key="s" :value="s">{{ ORDER_STATUS_LABELS[s] }}</option>
        </select>
      </div>
      <div>
        <label for="o-channel" class="label">Channel</label>
        <select id="o-channel" class="input" :value="list.filters.channel" @change="list.set('channel', ($event.target as HTMLSelectElement).value)">
          <option value="">Any</option>
          <option value="api">Integration key</option>
          <option value="manual">Entered by hand</option>
        </select>
      </div>
      <div>
        <label for="o-from" class="label">Ordered from</label>
        <input id="o-from" type="date" class="input" :value="fromDay" @change="list.set('from', dayStart(($event.target as HTMLInputElement).value))" />
      </div>
      <div>
        <label for="o-to" class="label">Ordered until</label>
        <input id="o-to" type="date" class="input" :value="toDay" @change="list.set('to', dayAfter(($event.target as HTMLInputElement).value))" />
      </div>
      <label class="flex items-center gap-2 text-sm text-slate-700 sm:col-span-2">
        <input type="checkbox" class="rounded border-slate-300" :checked="list.filters.needs_attention === 'true'" data-testid="filter-attention"
          @change="list.set('needs_attention', ($event.target as HTMLInputElement).checked ? 'true' : '')" />
        Only orders that need attention
      </label>
    </form>

    <div class="mb-2 flex flex-wrap items-center justify-between gap-2">
      <p class="text-sm text-slate-500" aria-live="polite">{{ list.total.value }} order{{ list.total.value === 1 ? '' : 's' }}</p>
      <button v-if="filtered" type="button" class="text-sm text-brand-700 hover:underline" @click="search = ''; $router.replace({ query: {} })">Clear filters</button>
    </div>

    <DataTable
      :columns="columns"
      :rows="list.items.value"
      :loading="list.loading.value"
      :error="list.error.value"
      :empty-title="filtered ? 'No orders match these filters' : 'No orders yet'"
      empty-message="Orders arrive through an integration key with the orders.write scope (Settings → Integrations) or are entered by hand."
      @retry="list.refetch()"
    >
      <template #cell-number="{ row }">
        <RouterLink :to="`/orders/${row.id}`" class="font-medium text-brand-700 hover:underline">{{ row.number }}</RouterLink>
        <p v-if="row.external_id" class="text-xs text-slate-500">{{ row.external_id }}</p>
        <p class="text-xs text-slate-600 sm:hidden">{{ row.customer.name }}</p>
      </template>
      <template #cell-customer="{ row }">
        <span class="block max-w-48 truncate">{{ row.customer.name || '—' }}</span>
        <span class="text-xs text-slate-500">{{ regionName(row.shipping_address.country) || row.shipping_address.country }}</span>
      </template>
      <template #cell-status="{ row }">
        <div class="flex flex-col items-start gap-1">
          <UiBadge :tone="orderTone(row.status)">{{ ORDER_STATUS_LABELS[row.status] ?? row.status }}</UiBadge>
          <span v-if="row.needs_attention && row.status !== 'needs_attention'" class="inline-flex items-center gap-1 text-xs font-medium text-red-700"><CircleAlert class="h-3.5 w-3.5" aria-hidden="true" /> Needs attention</span>
        </div>
      </template>
      <template #cell-lines="{ row }">
        {{ row.line_count }}<span v-if="row.unassigned_count" class="text-red-700"> ({{ row.unassigned_count }} unassigned)</span>
        <span class="block text-xs text-slate-500">{{ row.supplier_order_count }} supplier order{{ row.supplier_order_count === 1 ? '' : 's' }}</span>
      </template>
      <template #cell-total="{ row }"><span class="whitespace-nowrap">{{ formatMoney(row.total, row.currency) }}</span></template>
      <template #cell-source="{ row }">{{ row.channel === 'api' ? row.source : `By hand · ${row.source}` }}</template>
      <template #cell-ordered="{ row }"><time :datetime="row.ordered_at" :title="formatDateTime(row.ordered_at)" class="whitespace-nowrap">{{ relativeTime(row.ordered_at) }}</time></template>
    </DataTable>
    <UiPagination :total="list.total.value" :limit="list.limit.value" :offset="list.offset.value" @update:offset="(o) => list.set('offset', String(o))" />
  </div>
</template>
