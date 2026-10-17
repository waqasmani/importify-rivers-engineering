<script setup lang="ts">
// One supplier order inside a customer order: what it contains, its warnings, and the next step.
// Drafts are reviewed and confirmed (orders.place) in ConfirmSupplierOrderDialog; confirmed manual
// orders show the placement sheet; automated ones wait for the user's automation to report back.
import { computed, ref } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { CircleCheck, ClipboardList, ExternalLink, Pencil, RotateCcw, Truck, TriangleAlert, Undo2, UserRound, X } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiButton from '@/components/ui/UiButton.vue'
import StateViews from '@/components/ui/StateViews.vue'
import AddressView from './AddressView.vue'
import ConfirmSupplierOrderDialog from './ConfirmSupplierOrderDialog.vue'
import PlacementSheet from './PlacementSheet.vue'
import ShipmentFormModal from './ShipmentFormModal.vue'
import ShipToFormModal from './ShipToFormModal.vue'
import ReasonModal from './ReasonModal.vue'
import { useAuth } from '@/stores/auth'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { formatDateTime, formatMoney } from '@/lib/format'
import { PLACEMENT_LABELS, SO_STATUS_LABELS, optionsText, safeLink, soTone } from './order-utils'

type SO = Schemas['SupplierOrderView']

const props = defineProps<{ id: string }>()
const emit = defineEmits<{ changed: [] }>()
const auth = useAuth()
const qc = useQueryClient()
const toast = useToast()
const { confirm } = useConfirm()
const { errors: cancelErrors, capture: captureCancel, clear: clearCancel } = useFieldErrors()

const key = computed(() => ['supplier-order', props.id])
const q = useQuery({ queryKey: key, queryFn: ({ signal }) => api.get<SO>(`/supplier-orders/${props.id}`, undefined, signal) })
const so = computed(() => q.data.value)

const confirming = ref(false)
const placing = ref(false)
const shipping = ref(false)
const editing = ref(false)
const cancelling = ref(false)
const busy = ref(false)

function updated(next: SO) {
  qc.setQueryData(key.value, next)
  void qc.invalidateQueries({ queryKey: ['supplier-orders'] })
  emit('changed')
}

async function act(path: string, done: string, body?: object) {
  busy.value = true
  try {
    updated(await api.post<SO>(`/supplier-orders/${props.id}/${path}`, body))
    toast.success(done)
  } catch (e) {
    toast.error(errorMessage(e))
    void q.refetch()
  } finally {
    busy.value = false
  }
}

async function revert() {
  const ok = await confirm({
    title: `Withdraw the confirmation of ${so.value!.number}`,
    message: 'The supplier order becomes a draft again and needs a new confirmation before it can be placed.' +
      (so.value!.placement_method === 'automation' ? ' Your automation receives supplier_order.cancelled and must stop any placement in progress.' : ''),
    confirmLabel: 'Revert to draft',
    danger: true,
  })
  if (ok) await act('revert', 'Reverted to draft.')
}

async function retry() {
  const ok = await confirm({
    title: `Retry ${so.value!.number}`,
    message: so.value!.placement_method === 'automation'
      ? 'REA hands the same supplier order to your automation again (a new supplier_order.confirmed event). Make sure the failed attempt did not place it.'
      : 'The supplier order goes back to the placement queue.',
    confirmLabel: 'Retry placement',
  })
  if (ok) await act('retry', 'Handed off again.')
}

async function switchToManual() {
  const ok = await confirm({
    title: `Place ${so.value!.number} by hand`,
    message: 'Your automation no longer handles this supplier order. Place it on the supplier’s site yourself and record the supplier order number.',
    confirmLabel: 'Switch to manual placement',
  })
  if (ok) await act('switch-to-manual', 'Switched to manual placement.')
}

async function delivered() {
  const ok = await confirm({ title: `Mark ${so.value!.number} delivered`, message: 'Record that the supplier’s shipment arrived.', confirmLabel: 'Mark delivered' })
  if (ok) await act('delivered', 'Marked delivered.')
}

const needsReason = computed(() => so.value?.status === 'placed' || so.value?.status === 'shipped')
function openCancel() {
  clearCancel()
  cancelling.value = true
}
async function cancel(reason: string) {
  busy.value = true
  try {
    updated(await api.post<SO>(`/supplier-orders/${props.id}/cancel`, { reason } satisfies Schemas['CancelInput']))
    cancelling.value = false
    toast.success('Supplier order cancelled. Its lines need attention again.')
  } catch (e) {
    captureCancel(e)
  } finally {
    busy.value = false
  }
}

const supplier = computed(() => so.value?.supplier_name || so.value?.source_domain || 'Unknown source')
const canWrite = computed(() => auth.can('orders.write'))
const canPlace = computed(() => auth.can('orders.place'))
</script>

<template>
  <article class="card p-4" :id="`so-${id}`" :data-testid="`supplier-order-${id}`" :aria-label="so ? `Supplier order ${so.number}` : 'Supplier order'">
    <StateViews v-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <template v-else-if="so">
      <header class="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
        <div class="min-w-0">
          <h3 class="flex flex-wrap items-center gap-2 text-sm font-semibold text-slate-900">
            {{ so.number }} · {{ supplier }}
            <UiBadge :tone="soTone(so.status)" data-testid="so-status">{{ SO_STATUS_LABELS[so.status] ?? so.status }}</UiBadge>
            <UiBadge :tone="so.placement_method === 'automation' ? 'violet' : 'slate'">{{ PLACEMENT_LABELS[so.placement_method] }}</UiBadge>
          </h3>
          <p class="text-xs text-slate-500">
            {{ so.items.length }} item{{ so.items.length === 1 ? '' : 's' }} · expected {{ so.expected_cost_total ? formatMoney(so.expected_cost_total, so.cost_currency) : 'cost unknown' }}
            <span v-if="so.confirmed_at"> · confirmed {{ formatDateTime(so.confirmed_at) }} by {{ so.confirmed_by || 'unknown' }}</span>
            <span v-if="so.placed_at"> · placed {{ formatDateTime(so.placed_at) }} by {{ so.placed_by || 'unknown' }}</span>
          </p>
        </div>
        <div class="flex flex-wrap gap-2">
          <template v-if="so.status === 'draft'">
            <UiButton size="sm" variant="primary" data-testid="open-confirm" @click="confirming = true"><CircleCheck class="h-4 w-4" aria-hidden="true" /> Review and confirm</UiButton>
            <UiButton v-if="canWrite" size="sm" @click="editing = true"><Pencil class="h-4 w-4" aria-hidden="true" /> Edit ship-to</UiButton>
          </template>
          <template v-if="so.status === 'confirmed'">
            <UiButton v-if="so.placement_method === 'manual'" size="sm" variant="primary" data-testid="open-placement" @click="placing = !placing">
              <ClipboardList class="h-4 w-4" aria-hidden="true" /> {{ placing ? 'Hide placement sheet' : 'Placement sheet' }}
            </UiButton>
            <UiButton v-if="canPlace" size="sm" :disabled="busy" @click="revert"><Undo2 class="h-4 w-4" aria-hidden="true" /> Revert to draft</UiButton>
          </template>
          <template v-if="so.status === 'failed'">
            <UiButton v-if="canPlace" size="sm" variant="primary" :disabled="busy" @click="retry"><RotateCcw class="h-4 w-4" aria-hidden="true" /> Retry</UiButton>
            <UiButton v-if="canWrite && so.placement_method === 'automation'" size="sm" :disabled="busy" @click="switchToManual"><UserRound class="h-4 w-4" aria-hidden="true" /> Place by hand</UiButton>
            <UiButton v-if="canPlace" size="sm" :disabled="busy" @click="revert"><Undo2 class="h-4 w-4" aria-hidden="true" /> Revert to draft</UiButton>
          </template>
          <template v-if="canWrite && (so.status === 'placed' || so.status === 'shipped')">
            <UiButton size="sm" :variant="so.status === 'placed' ? 'primary' : 'secondary'" data-testid="open-shipment" @click="shipping = true">
              <Truck class="h-4 w-4" aria-hidden="true" /> {{ so.status === 'placed' ? 'Record shipment' : 'Update tracking' }}
            </UiButton>
            <UiButton v-if="so.status === 'shipped'" size="sm" variant="primary" :disabled="busy" @click="delivered"><CircleCheck class="h-4 w-4" aria-hidden="true" /> Mark delivered</UiButton>
          </template>
          <UiButton v-if="canWrite && so.status !== 'cancelled' && so.status !== 'delivered'" size="sm" variant="ghost" :disabled="busy" @click="openCancel">
            <X class="h-4 w-4" aria-hidden="true" /> {{ needsReason ? 'Mark cancelled' : 'Cancel' }}
          </UiButton>
        </div>
      </header>

      <p v-if="so.status === 'confirmed' && so.placement_method === 'automation'" class="mt-3 rounded-md bg-violet-50 p-3 text-sm text-violet-900">
        Handed to your automation (attempt {{ so.attempt }}). It reports the placement or a failure with an integration key.
      </p>
      <p v-if="so.status === 'failed'" class="mt-3 rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-800" role="status" data-testid="so-failure">
        Placement failed{{ so.failed_at ? ` (${formatDateTime(so.failed_at)})` : '' }}: {{ so.failure_reason }}
      </p>
      <p v-if="so.status === 'cancelled'" class="mt-3 text-sm text-slate-600">Cancelled {{ formatDateTime(so.cancelled_at) }}<span v-if="so.cancelled_by"> by {{ so.cancelled_by }}</span><span v-if="so.cancel_reason">: {{ so.cancel_reason }}</span></p>

      <div v-if="so.warnings.length && so.status === 'draft'" class="mt-3 rounded-md border border-amber-300 bg-amber-50 p-3" data-testid="so-warnings">
        <p class="flex items-center gap-1.5 text-sm font-medium text-amber-900"><TriangleAlert class="h-4 w-4" aria-hidden="true" /> {{ so.warnings.length }} warning{{ so.warnings.length === 1 ? '' : 's' }} to review before confirming</p>
        <ul class="mt-1 list-disc pl-5 text-sm text-amber-900">
          <li v-for="(w, i) in so.warnings" :key="`${w.code}-${i}`">{{ w.message }}</li>
        </ul>
      </div>

      <div v-if="!placing" class="mt-3 grid gap-4 lg:grid-cols-3">
        <ul class="divide-y divide-slate-100 lg:col-span-2" aria-label="Items">
          <li v-for="it in so.items" :key="it.line_id" class="flex flex-col gap-0.5 py-1.5 sm:flex-row sm:items-start sm:justify-between">
            <div class="min-w-0">
              <p class="text-sm break-words text-slate-800">{{ it.quantity }} × {{ it.title }}<span v-if="it.variant_title" class="text-slate-600"> ({{ it.variant_title }})</span></p>
              <p class="text-xs break-words text-slate-500">{{ [it.sku, optionsText(it.options)].filter(Boolean).join(' · ') }}</p>
            </div>
            <div class="flex shrink-0 items-center gap-2 text-xs text-slate-600">
              <span>{{ it.expected_unit_cost ? formatMoney(it.expected_unit_cost, it.cost_currency) : 'no cost' }}</span>
              <a v-if="safeLink(it.source_url)" :href="safeLink(it.source_url)" target="_blank" rel="noopener noreferrer nofollow" class="inline-flex items-center gap-1 text-brand-700 hover:underline" :aria-label="`Source page of ${it.title}`">
                <ExternalLink class="h-3.5 w-3.5" aria-hidden="true" /> Source
              </a>
            </div>
          </li>
        </ul>
        <div class="space-y-3">
          <div>
            <h4 class="mb-1 text-xs font-semibold tracking-wide text-slate-500 uppercase">Ship to</h4>
            <AddressView :address="so.ship_to" />
          </div>
          <div v-if="so.supplier_order_number || so.actual_cost">
            <h4 class="mb-1 text-xs font-semibold tracking-wide text-slate-500 uppercase">Placement</h4>
            <p class="text-sm text-slate-700">Supplier order {{ so.supplier_order_number || '—' }}<span v-if="so.actual_cost"> · paid {{ formatMoney(so.actual_cost, so.actual_currency) }}</span></p>
            <p v-if="so.placement_note" class="text-xs whitespace-pre-line text-slate-500">{{ so.placement_note }}</p>
          </div>
          <div v-if="so.tracking_number" data-testid="so-tracking">
            <h4 class="mb-1 text-xs font-semibold tracking-wide text-slate-500 uppercase">Tracking</h4>
            <p class="text-sm text-slate-700">{{ [so.carrier, so.tracking_number].filter(Boolean).join(' · ') }}</p>
            <a v-if="safeLink(so.tracking_url)" :href="safeLink(so.tracking_url)" target="_blank" rel="noopener noreferrer nofollow" class="text-xs text-brand-700 hover:underline">Track the parcel</a>
          </div>
        </div>
      </div>

      <div v-if="placing && so.status === 'confirmed'" class="mt-4 border-t border-slate-200 pt-4">
        <PlacementSheet :supplier-order="so" @placed="(next) => { placing = false; updated(next) }" />
      </div>

      <ConfirmSupplierOrderDialog :open="confirming" :supplier-order="so" @close="confirming = false" @changed="q.refetch()"
        @confirmed="(next) => { confirming = false; updated(next) }" />
      <ShipmentFormModal :open="shipping" :supplier-order="so" @close="shipping = false" @saved="(next) => { shipping = false; updated(next) }" />
      <ShipToFormModal :open="editing" :supplier-order="so" @close="editing = false" @saved="(next) => { editing = false; updated(next) }" />
      <ReasonModal
        :open="cancelling"
        :title="`Cancel ${so.number}`"
        :message="needsReason
          ? 'Mark this supplier order cancelled in REA, for example because the supplier cancelled it. REA does not contact the supplier. Its lines need attention again.'
          : 'Cancel this supplier order in REA. Its lines need attention again: map them to another product, prepare them again or cancel them.'"
        :confirm-label="needsReason ? 'Mark cancelled' : 'Cancel supplier order'"
        :required="needsReason"
        :saving="busy"
        :error="cancelErrors.reason"
        @close="cancelling = false"
        @submit="cancel"
      />
    </template>
  </article>
</template>
