<script setup lang="ts">
// One customer order: customer and shipping address, the lines as sold and what they resolved to,
// the supplier orders REA prepared (each confirmed separately by a person), and the timeline.
import { computed, nextTick, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { CircleAlert, Eraser, RefreshCw, X } from 'lucide-vue-next'
import { ApiError, api, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import StateViews from '@/components/ui/StateViews.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiButton from '@/components/ui/UiButton.vue'
import DetailFacts, { type Fact } from '@/components/crm/DetailFacts.vue'
import AddressView from '@/components/orders/AddressView.vue'
import SupplierOrderCard from '@/components/orders/SupplierOrderCard.vue'
import LineMapModal from '@/components/orders/LineMapModal.vue'
import ReasonModal from '@/components/orders/ReasonModal.vue'
import { useAuth } from '@/stores/auth'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { formatDateTime, formatMoney, relativeTime } from '@/lib/format'
import { ORDER_STATUS_LABELS, RESOLUTION_LABELS, isOpen, optionsText, orderTone, resolutionTone } from '@/components/orders/order-utils'

type Detail = Schemas['OrderDetail']
type Line = Schemas['OrderLineView']

const props = defineProps<{ id: string }>()
const route = useRoute()
const auth = useAuth()
const qc = useQueryClient()
const toast = useToast()
const { confirm } = useConfirm()
const { errors: cancelErrors, capture: captureCancel, clear: clearCancel } = useFieldErrors()

const key = computed(() => ['order', props.id])
const q = useQuery({ queryKey: key, queryFn: ({ signal }) => api.get<Detail>(`/orders/${props.id}`, undefined, signal) })
const o = computed(() => q.data.value)
const notFound = computed(() => q.error.value instanceof ApiError && q.error.value.status === 404)
const canWrite = computed(() => auth.can('orders.write'))
// Links from the supplier order queues point at one card (#so-<id>).
watch(
  () => !!o.value,
  async (loaded) => {
    if (!loaded || !route.hash.startsWith('#so-')) return
    await nextTick()
    document.getElementById(route.hash.slice(1))?.scrollIntoView({ block: 'start' })
  },
  { immediate: true },
)

function set(d: Detail) {
  qc.setQueryData(key.value, d)
  void qc.invalidateQueries({ queryKey: ['orders'] })
  void qc.invalidateQueries({ queryKey: ['supplier-orders'] })
  for (const so of d.supplier_orders) void qc.invalidateQueries({ queryKey: ['supplier-order', so.id] })
}
function refresh() {
  void qc.invalidateQueries({ queryKey: key.value })
  void qc.invalidateQueries({ queryKey: ['orders'] })
}

const soNumber = (id?: string | null) => o.value?.supplier_orders.find((s) => s.id === id)?.number ?? ''
const soStatus = (id?: string | null) => o.value?.supplier_orders.find((s) => s.id === id)?.status ?? ''
const lineEditable = (l: Line) => !o.value?.cancelled_at && !l.cancelled_at && (!l.supplier_order_id || soStatus(l.supplier_order_id) === 'draft')
const openSupplierOrders = computed(() => (o.value?.supplier_orders ?? []).filter((s) => isOpen(s.status)))
const shownSupplierOrders = computed(() => (o.value?.supplier_orders ?? []).filter((s) => s.status !== 'cancelled' || showCancelled.value))
const cancelledCount = computed(() => (o.value?.supplier_orders ?? []).filter((s) => s.status === 'cancelled').length)
const showCancelled = ref(false)
const anyPlaced = computed(() => (o.value?.supplier_orders ?? []).some((s) => ['placed', 'shipped', 'delivered'].includes(s.status)))

const facts = computed<Fact[]>(() => {
  const x = o.value
  if (!x) return []
  return [
    { label: 'Received through', value: x.channel === 'api' ? `Integration key ${x.source}` : `Entered by ${x.source}` },
    { label: 'External order ID', value: x.external_id },
    { label: 'Ordered', value: formatDateTime(x.ordered_at) },
    { label: 'Customer paid', value: x.total ? formatMoney(x.total, x.currency) : null },
    { label: 'Shipping and tax', value: [x.shipping_amount ? `shipping ${formatMoney(x.shipping_amount, x.currency)}` : '', x.tax_amount ? `tax ${formatMoney(x.tax_amount, x.currency)}` : ''].filter(Boolean).join(', ') },
    { label: 'Currency', value: x.currency },
  ]
})

/** Status changes read better with the labels shown elsewhere. */
function eventText(ev: Schemas['OrderEventView']): string {
  if (ev.kind === 'status_changed') return `Status: ${ORDER_STATUS_LABELS[ev.from_status] ?? ev.from_status} → ${ORDER_STATUS_LABELS[ev.to_status] ?? ev.to_status}`
  return ev.summary
}

const mapping = ref<Line | null>(null)
const busy = ref(false)
async function prepare() {
  busy.value = true
  try {
    set(await api.post<Detail>(`/orders/${props.id}/prepare`))
    toast.success('Draft supplier orders prepared again with the current product data.')
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    busy.value = false
  }
}
async function cancelLine(l: Line) {
  const ok = await confirm({ title: `Cancel line ${l.position + 1}`, message: 'This line will not be ordered from any supplier.', confirmLabel: 'Cancel line', danger: true })
  if (!ok) return
  try {
    set(await api.post<Detail>(`/orders/${props.id}/lines/${l.id}/cancel`))
    toast.success('Line cancelled.')
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

const cancelling = ref(false)
async function cancelOrder(reason: string) {
  busy.value = true
  try {
    set(await api.post<Detail>(`/orders/${props.id}/cancel`, { reason } satisfies Schemas['CancelInput']))
    cancelling.value = false
    toast.success('Order cancelled.')
  } catch (e) {
    captureCancel(e)
  } finally {
    busy.value = false
  }
}

async function erase() {
  const ok = await confirm({
    title: 'Erase the customer’s personal data',
    message: 'Name, e-mail, phone, shipping address and notes are replaced with “[erased]” in this order, its supplier orders and stored webhook payloads. This cannot be undone.',
    confirmLabel: 'Erase customer data',
    danger: true,
  })
  if (!ok) return
  try {
    set(await api.post<Detail>(`/orders/${props.id}/erase-customer`))
    toast.success('Customer data erased.')
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
</script>

<template>
  <div>
    <StateViews v-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="notFound" kind="empty" title="Order not found" message="It may have been removed, or the link is wrong." />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <template v-else-if="o">
      <PageHeader :title="`Order ${o.number}`" back="/orders">
        <template #meta>
          <p class="mt-1 flex flex-wrap items-center gap-2">
            <UiBadge :tone="orderTone(o.status)" data-testid="order-status">{{ ORDER_STATUS_LABELS[o.status] ?? o.status }}</UiBadge>
            <span v-if="o.needs_attention && o.status !== 'needs_attention'" class="inline-flex items-center gap-1 text-xs font-medium text-red-700" data-testid="needs-attention">
              <CircleAlert class="h-3.5 w-3.5" aria-hidden="true" /> Needs attention
            </span>
            <span class="text-xs text-slate-500">received {{ relativeTime(o.created_at) }}</span>
          </p>
        </template>
        <template #actions>
          <UiButton v-if="canWrite && !o.cancelled_at" :loading="busy" @click="prepare"><RefreshCw class="h-4 w-4" aria-hidden="true" /> Prepare again</UiButton>
          <UiButton v-if="canWrite && !o.cancelled_at && !anyPlaced" variant="ghost" @click="clearCancel(); cancelling = true"><X class="h-4 w-4" aria-hidden="true" /> Cancel order</UiButton>
          <UiButton v-if="canWrite && !o.customer_erased_at" variant="ghost" :disabled="openSupplierOrders.length > 0"
            :title="openSupplierOrders.length ? 'Possible once no supplier order waits for confirmation or placement' : undefined" @click="erase">
            <Eraser class="h-4 w-4" aria-hidden="true" /> Erase customer data
          </UiButton>
        </template>
      </PageHeader>

      <p v-if="o.cancelled_at" class="mb-4 rounded-md border border-slate-200 bg-slate-100 p-3 text-sm text-slate-700">Cancelled {{ formatDateTime(o.cancelled_at) }}<span v-if="o.cancel_reason">: {{ o.cancel_reason }}</span></p>

      <div class="grid gap-4 lg:grid-cols-3">
        <section class="card p-4 lg:col-span-2" aria-label="Order facts"><DetailFacts :facts="facts" /></section>
        <section class="card p-4" aria-labelledby="customer-heading">
          <h2 id="customer-heading" class="mb-2 text-sm font-semibold text-slate-900">Customer</h2>
          <p v-if="o.customer_erased_at" class="mb-2 text-xs text-slate-500">Personal data erased {{ formatDateTime(o.customer_erased_at) }}.</p>
          <p class="text-sm text-slate-800">{{ o.customer.name || '—' }}</p>
          <p v-if="o.customer.email" class="text-sm break-all text-slate-600">{{ o.customer.email }}</p>
          <p v-if="o.customer.phone" class="text-sm text-slate-600">{{ o.customer.phone }}</p>
          <h3 class="mt-3 mb-1 text-xs font-semibold tracking-wide text-slate-500 uppercase">Shipping address</h3>
          <AddressView :address="o.shipping_address" />
          <p v-if="o.customer_note" class="mt-3 text-sm whitespace-pre-line text-slate-600"><span class="font-medium">Note:</span> {{ o.customer_note }}</p>
        </section>
      </div>

      <section class="card mt-4 overflow-hidden" aria-labelledby="lines-heading">
        <h2 id="lines-heading" class="border-b border-slate-200 px-4 py-3 text-sm font-semibold text-slate-900">Lines</h2>
        <ul class="divide-y divide-slate-100">
          <li v-for="l in o.lines" :key="l.id" class="flex flex-col gap-2 px-4 py-3 sm:flex-row sm:items-start sm:justify-between" :data-testid="`line-${l.position}`">
            <div class="min-w-0">
              <p class="text-sm font-medium break-words text-slate-800" :class="l.cancelled_at ? 'line-through' : ''">
                {{ l.quantity }} × {{ l.title || l.sku || 'Untitled line' }}
                <span v-if="l.unit_price" class="font-normal text-slate-500">at {{ formatMoney(l.unit_price, o.currency) }}</span>
              </p>
              <p class="text-xs break-words text-slate-500">{{ [l.sku ? `SKU ${l.sku}` : '', l.external_line_id ? `line ${l.external_line_id}` : ''].filter(Boolean).join(' · ') }}</p>
              <p class="mt-1 flex flex-wrap items-center gap-2 text-xs">
                <UiBadge v-if="l.cancelled_at" tone="slate">Cancelled</UiBadge>
                <UiBadge v-else :tone="resolutionTone(l.resolution)">{{ RESOLUTION_LABELS[l.resolution] ?? l.resolution }}</UiBadge>
                <RouterLink v-if="l.product_id && auth.can('products.read')" :to="`/products/${l.product_id}`" class="text-brand-700 hover:underline">
                  {{ l.sourcing?.product_title || 'Product' }}<span v-if="l.sourcing?.variant_title"> ({{ l.sourcing.variant_title }})</span>
                </RouterLink>
                <span v-if="l.sourcing && optionsText(l.sourcing.options)" class="text-slate-500">{{ optionsText(l.sourcing.options) }}</span>
                <a v-if="l.supplier_order_id" :href="`#so-${l.supplier_order_id}`" class="text-slate-600 hover:underline">in {{ soNumber(l.supplier_order_id) }}</a>
              </p>
              <p v-if="l.resolution === 'unmatched' && !l.cancelled_at" class="mt-1 text-xs text-red-700">{{ l.resolution_note }}</p>
            </div>
            <div v-if="canWrite && lineEditable(l)" class="flex shrink-0 gap-2">
              <UiButton size="sm" :variant="l.resolution === 'unmatched' ? 'primary' : 'secondary'" @click="mapping = l">{{ l.resolution === 'unmatched' ? 'Map to a product' : 'Change' }}</UiButton>
              <UiButton size="sm" variant="ghost" @click="cancelLine(l)">Cancel line</UiButton>
            </div>
          </li>
        </ul>
      </section>

      <section class="mt-6" aria-labelledby="so-heading">
        <div class="mb-2 flex flex-wrap items-center justify-between gap-2">
          <h2 id="so-heading" class="text-sm font-semibold text-slate-900">Supplier orders</h2>
          <label v-if="cancelledCount" class="flex items-center gap-2 text-xs text-slate-600">
            <input v-model="showCancelled" type="checkbox" class="rounded border-slate-300" /> Show {{ cancelledCount }} cancelled
          </label>
        </div>
        <p class="mb-3 text-xs text-slate-500">REA prepares these automatically. Each is handed off for placement only after a person with the orders.place permission confirms it; nothing is confirmed automatically.</p>
        <StateViews v-if="shownSupplierOrders.length === 0" kind="empty" title="No supplier orders" message="Map the lines to products, then prepare again." />
        <div class="space-y-4">
          <SupplierOrderCard v-for="so in shownSupplierOrders" :id="so.id" :key="so.id" @changed="refresh" />
        </div>
      </section>

      <section class="card mt-6 p-4" aria-labelledby="timeline-heading">
        <h2 id="timeline-heading" class="mb-2 text-sm font-semibold text-slate-900">Timeline</h2>
        <ol class="space-y-2" data-testid="order-timeline">
          <li v-for="ev in o.events" :key="ev.id" class="flex flex-col text-sm sm:flex-row sm:gap-3">
            <time :datetime="ev.created_at" class="shrink-0 text-xs text-slate-500 sm:w-40">{{ formatDateTime(ev.created_at) }}</time>
            <span class="text-slate-700">{{ eventText(ev) }} <span class="text-xs text-slate-500">· {{ ev.actor }}</span></span>
          </li>
        </ol>
      </section>

      <LineMapModal v-if="mapping" :open="!!mapping" :order-id="o.id" :line="mapping" @close="mapping = null" @saved="(d) => { mapping = null; set(d) }" />
      <ReasonModal
        :open="cancelling"
        :title="`Cancel order ${o.number}`"
        message="Cancels the order and its supplier orders that are not placed yet. REA contacts no one; your automation is told about confirmed orders it may be working on."
        confirm-label="Cancel order"
        :saving="busy"
        :error="cancelErrors.reason"
        @close="cancelling = false"
        @submit="cancelOrder"
      />
    </template>
  </div>
</template>
