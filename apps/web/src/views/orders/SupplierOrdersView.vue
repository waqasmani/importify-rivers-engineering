<script setup lang="ts">
// Work queues for supplier orders: drafts to confirm, confirmed orders to place, placed orders
// waiting for tracking and failed placements. Each row opens its customer order.
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { refDebounced } from '@vueuse/core'
import { useQuery } from '@tanstack/vue-query'
import { TriangleAlert } from 'lucide-vue-next'
import { api, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import DataTable, { type Column } from '@/components/ui/DataTable.vue'
import UiPagination from '@/components/ui/UiPagination.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiTabs from '@/components/ui/UiTabs.vue'
import { useListQuery } from '@/composables/useListQuery'
import { formatDateTime, formatMoney, relativeTime } from '@/lib/format'
import { PLACEMENT_LABELS, SO_STATUS_LABELS, soTone } from '@/components/orders/order-utils'

type Row = Schemas['SupplierOrderListItem']
const QUEUES = [
  { key: 'to_confirm', label: 'To confirm', help: 'Drafts prepared from customer orders. Review each and confirm it to hand it off for placement.' },
  { key: 'to_place', label: 'To place', help: 'Confirmed. Manual orders are placed by a person from the placement sheet; automated ones wait for your automation to report back.' },
  { key: 'awaiting_tracking', label: 'Awaiting tracking', help: 'Placed with the supplier. Record the shipment when tracking arrives.' },
  { key: 'failed', label: 'Failed', help: 'Your automation reported that placing these failed. Retry, place by hand, revert or cancel.' },
] as const

const router = useRouter()
const list = useListQuery<Row>('supplier-orders', '/supplier-orders', ['queue', 'q'], { queue: 'to_confirm' })
const queue = computed({
  get: () => list.filters.queue || 'to_confirm',
  set: (v: string) => list.set('queue', v),
})
const counts = useQuery({ queryKey: ['supplier-orders', 'queues'], queryFn: ({ signal }) => api.get<Schemas['SupplierOrderQueuesDoc']>('/supplier-orders/queues', undefined, signal) })
const tabs = computed(() => QUEUES.map((t) => ({ key: t.key, label: t.label, count: counts.data.value ? counts.data.value[t.key] : undefined })))
const help = computed(() => QUEUES.find((t) => t.key === queue.value)?.help ?? '')

const search = ref(list.filters.q ?? '')
const debounced = refDebounced(search, 300)
watch(debounced, (v) => {
  if (v.trim() !== (list.filters.q ?? '')) list.set('q', v.trim())
})

const columns: Column[] = [
  { key: 'number', label: 'Supplier order' },
  { key: 'supplier', label: 'Supplier' },
  { key: 'items', label: 'Items', class: 'hidden md:table-cell' },
  { key: 'cost', label: 'Expected cost', class: 'hidden md:table-cell text-right' },
  { key: 'status', label: 'Status', class: 'hidden sm:table-cell' },
  { key: 'updated', label: 'Updated', class: 'hidden lg:table-cell' },
]
const open = (r: Row) => router.push({ path: `/orders/${r.order_id}`, hash: `#so-${r.id}` })
</script>

<template>
  <div>
    <PageHeader title="Supplier orders" subtitle="Purchase orders REA prepared from customer orders. Nothing is handed off for placement until a person confirms it." />
    <UiTabs v-model="queue" :tabs="tabs">
      <p class="mb-3 text-sm text-slate-600">{{ help }}</p>
      <div class="mb-3 max-w-md">
        <label for="so-q" class="label">Search</label>
        <input id="so-q" v-model="search" type="search" class="input" placeholder="PO number, order number, supplier or supplier order number" />
      </div>
      <DataTable
        :columns="columns"
        :rows="list.items.value"
        :loading="list.loading.value"
        :error="list.error.value"
        empty-title="Nothing in this queue"
        empty-message="Supplier orders appear here as customer orders come in and move through confirmation, placement and shipment."
        @retry="list.refetch()"
      >
        <template #cell-number="{ row }">
          <button type="button" class="font-medium text-brand-700 hover:underline" @click="open(row)">{{ row.number }}</button>
          <p class="text-xs text-slate-500">for {{ row.order_number }}<span v-if="row.order_external_id"> ({{ row.order_external_id }})</span></p>
          <p v-if="row.warning_count" class="mt-0.5 inline-flex items-center gap-1 text-xs font-medium text-amber-800"><TriangleAlert class="h-3.5 w-3.5" aria-hidden="true" /> {{ row.warning_count }} warning{{ row.warning_count === 1 ? '' : 's' }}</p>
        </template>
        <template #cell-supplier="{ row }">
          <span class="block max-w-48 truncate">{{ row.supplier_name || row.source_domain || 'Unknown source' }}</span>
          <span class="text-xs text-slate-500">{{ PLACEMENT_LABELS[row.placement_method] }}</span>
        </template>
        <template #cell-items="{ row }">{{ row.item_count }} ({{ row.quantity }} units)</template>
        <template #cell-cost="{ row }"><span class="whitespace-nowrap">{{ row.expected_cost_total ? formatMoney(row.expected_cost_total, row.cost_currency) : '—' }}</span></template>
        <template #cell-status="{ row }">
          <UiBadge :tone="soTone(row.status)">{{ SO_STATUS_LABELS[row.status] ?? row.status }}</UiBadge>
          <p v-if="row.failure_reason" class="mt-0.5 max-w-56 truncate text-xs text-red-700" :title="row.failure_reason">{{ row.failure_reason }}</p>
        </template>
        <template #cell-updated="{ row }"><time :datetime="row.updated_at" :title="formatDateTime(row.updated_at)">{{ relativeTime(row.updated_at) }}</time></template>
      </DataTable>
      <UiPagination :total="list.total.value" :limit="list.limit.value" :offset="list.offset.value" @update:offset="(o) => list.set('offset', String(o))" />
    </UiTabs>
  </div>
</template>
