<script setup lang="ts">
// Review and confirm a draft supplier order. It lists exactly what will be handed off (items,
// quantities, options, source links, expected cost, ship-to, placement method, note) and sends the
// content hash of what is shown, so the server refuses the confirmation if anything changed in the
// meantime. With warnings, "I reviewed the warnings" must be ticked. Only people with orders.place
// can confirm; nothing is ever confirmed automatically.
import { computed, ref, watch } from 'vue'
import { ExternalLink, TriangleAlert } from 'lucide-vue-next'
import { ApiError, api, errorMessage, type Schemas } from '@/api/client'
import UiModal from '@/components/ui/UiModal.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import AddressView from './AddressView.vue'
import { useAuth } from '@/stores/auth'
import { useToast } from '@/composables/useToast'
import { formatMoney } from '@/lib/format'
import { PLACEMENT_HELP, PLACEMENT_LABELS, lineCost, optionsText, safeLink } from './order-utils'

type SO = Schemas['SupplierOrderView']

const props = defineProps<{ open: boolean; supplierOrder: SO }>()
const emit = defineEmits<{ close: []; confirmed: [so: SO]; changed: [] }>()
const auth = useAuth()
const toast = useToast()

const acknowledged = ref(false)
const saving = ref(false)
const problem = ref('')
// A new review starts unacknowledged; so do changed contents (the warnings may be new).
watch(
  () => props.open,
  (open) => {
    acknowledged.value = false
    if (open) problem.value = ''
  },
)
watch(
  () => props.supplierOrder.content_hash,
  () => (acknowledged.value = false),
)

const so = computed(() => props.supplierOrder)
const canPlace = computed(() => auth.can('orders.place'))
const hasWarnings = computed(() => so.value.warnings.length > 0)
const supplier = computed(() => so.value.supplier_name || so.value.source_domain || 'Unknown source')
const canSubmit = computed(() => canPlace.value && so.value.status === 'draft' && (!hasWarnings.value || acknowledged.value) && !saving.value)

async function submit() {
  if (!canSubmit.value) return
  saving.value = true
  problem.value = ''
  try {
    const body: Schemas['ConfirmInput'] = { content_hash: so.value.content_hash, acknowledge_warnings: hasWarnings.value && acknowledged.value }
    const res = await api.post<SO>(`/supplier-orders/${so.value.id}/confirm`, body)
    toast.success(`${res.number} confirmed for ${PLACEMENT_LABELS[res.placement_method]?.toLowerCase() ?? res.placement_method}.`)
    emit('confirmed', res)
  } catch (e) {
    if (e instanceof ApiError && e.code === 'content_changed') {
      problem.value = 'This supplier order changed since you opened it. The updated contents are shown below; review them again before confirming.'
      emit('changed')
    } else {
      problem.value = errorMessage(e)
    }
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <UiModal :open="open" :title="`Confirm ${so.number}`" size="lg" @close="emit('close')">
    <div class="space-y-4" data-testid="confirm-dialog">
      <p class="text-sm text-slate-700">
        Confirming hands this purchase order off for placement. REA itself never buys anything or contacts the supplier.
        <strong class="font-medium">{{ PLACEMENT_LABELS[so.placement_method] }}:</strong> {{ PLACEMENT_HELP[so.placement_method] }}
      </p>
      <p v-if="problem" class="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-800" role="alert" data-testid="confirm-problem">{{ problem }}</p>

      <dl class="grid gap-3 text-sm sm:grid-cols-3">
        <div><dt class="text-xs text-slate-500">Supplier</dt><dd class="font-medium text-slate-800">{{ supplier }}</dd></div>
        <div><dt class="text-xs text-slate-500">Placement</dt><dd><UiBadge :tone="so.placement_method === 'automation' ? 'violet' : 'blue'">{{ PLACEMENT_LABELS[so.placement_method] }}</UiBadge></dd></div>
        <div>
          <dt class="text-xs text-slate-500">Expected cost</dt>
          <dd class="font-medium text-slate-800" data-testid="confirm-total">{{ so.expected_cost_total ? formatMoney(so.expected_cost_total, so.cost_currency) : 'Unknown (see warnings)' }}</dd>
        </div>
      </dl>

      <section aria-labelledby="confirm-items">
        <h3 id="confirm-items" class="mb-1 text-xs font-semibold tracking-wide text-slate-500 uppercase">Items</h3>
        <ul class="divide-y divide-slate-100 rounded-md border border-slate-200" data-testid="confirm-items">
          <li v-for="it in so.items" :key="it.line_id" class="flex flex-col gap-1 px-3 py-2 sm:flex-row sm:items-start sm:justify-between">
            <div class="min-w-0">
              <p class="text-sm font-medium break-words text-slate-800">{{ it.quantity }} × {{ it.title }}<span v-if="it.variant_title" class="font-normal text-slate-600"> ({{ it.variant_title }})</span></p>
              <p class="text-xs break-words text-slate-500">{{ [it.sku ? `SKU ${it.sku}` : '', optionsText(it.options)].filter(Boolean).join(' · ') || '—' }}</p>
              <a v-if="safeLink(it.source_url)" :href="safeLink(it.source_url)" target="_blank" rel="noopener noreferrer nofollow" class="inline-flex max-w-full items-center gap-1 text-xs break-all text-brand-700 hover:underline">
                <ExternalLink class="h-3 w-3 shrink-0" aria-hidden="true" />{{ it.source_url }}
              </a>
              <p v-else class="text-xs text-red-700">No source link</p>
            </div>
            <p class="shrink-0 text-sm text-slate-700 sm:text-right">
              {{ it.expected_unit_cost ? `${formatMoney(it.expected_unit_cost, it.cost_currency)} each` : 'No cost' }}
              <span v-if="it.expected_unit_cost" class="block text-xs text-slate-500">{{ formatMoney(lineCost(it.expected_unit_cost, it.quantity), it.cost_currency) }}</span>
            </p>
          </li>
        </ul>
      </section>

      <div class="grid gap-4 sm:grid-cols-2">
        <section aria-labelledby="confirm-ship">
          <h3 id="confirm-ship" class="mb-1 text-xs font-semibold tracking-wide text-slate-500 uppercase">Ship to</h3>
          <AddressView :address="so.ship_to" />
        </section>
        <section aria-labelledby="confirm-note">
          <h3 id="confirm-note" class="mb-1 text-xs font-semibold tracking-wide text-slate-500 uppercase">Note to the supplier</h3>
          <p class="text-sm whitespace-pre-line text-slate-700">{{ so.note_to_supplier || '—' }}</p>
        </section>
      </div>

      <section v-if="hasWarnings" class="space-y-2 rounded-md border border-amber-300 bg-amber-50 p-3" aria-labelledby="confirm-warnings" data-testid="confirm-warnings">
        <h3 id="confirm-warnings" class="flex items-center gap-1.5 text-sm font-semibold text-amber-900"><TriangleAlert class="h-4 w-4" aria-hidden="true" /> {{ so.warnings.length }} warning{{ so.warnings.length === 1 ? '' : 's' }}</h3>
        <ul class="list-disc space-y-1 pl-5 text-sm text-amber-900">
          <li v-for="(w, i) in so.warnings" :key="`${w.code}-${i}`">{{ w.message }}</li>
        </ul>
        <label class="flex items-start gap-2 text-sm font-medium text-amber-950">
          <input v-model="acknowledged" type="checkbox" class="mt-0.5 rounded border-amber-400" data-testid="confirm-ack" :disabled="!canPlace" />
          I reviewed the warnings and want to confirm anyway.
        </label>
      </section>

      <p v-if="!canPlace" class="rounded-md border border-slate-200 bg-slate-50 p-3 text-sm text-slate-700" data-testid="confirm-no-permission">
        Only people with the <code class="text-xs">orders.place</code> permission can confirm supplier orders. Ask an administrator to confirm this one.
      </p>
    </div>
    <template #footer>
      <UiButton variant="ghost" @click="emit('close')">Cancel</UiButton>
      <UiButton variant="primary" :loading="saving" :disabled="!canSubmit" data-testid="confirm-submit" @click="submit">Confirm and hand off</UiButton>
    </template>
  </UiModal>
</template>
