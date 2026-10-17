<script setup lang="ts">
// Manual placement of a confirmed supplier order: everything needed to order it on the supplier's
// site (source links, options, quantities, expected cost, ship-to with copy buttons) and a form to
// record the supplier's order number and the actual cost afterwards. REA does not place anything.
import { reactive, ref, watch } from 'vue'
import { ExternalLink } from 'lucide-vue-next'
import { api, type Schemas } from '@/api/client'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import CopyButton from '@/components/settings/CopyButton.vue'
import AddressView from './AddressView.vue'
import { useAuth } from '@/stores/auth'
import { useToast } from '@/composables/useToast'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { formatMoney } from '@/lib/format'
import { lineCost, optionsText, safeLink } from './order-utils'

type SO = Schemas['SupplierOrderView']

const props = defineProps<{ supplierOrder: SO }>()
const emit = defineEmits<{ placed: [so: SO] }>()
const auth = useAuth()
const toast = useToast()
const { errors, capture, clear } = useFieldErrors()
const saving = ref(false)
const form = reactive({ supplier_order_number: '', actual_cost: '', currency: '', note: '' })
watch(
  () => props.supplierOrder.id,
  () => {
    clear()
    Object.assign(form, { supplier_order_number: '', actual_cost: '', currency: props.supplierOrder.cost_currency ?? '', note: '' })
  },
  { immediate: true },
)

async function submit() {
  saving.value = true
  try {
    const cost = form.actual_cost.trim()
    const body: Schemas['PlacedInput'] = { supplier_order_number: form.supplier_order_number.trim(), note: form.note }
    if (cost) {
      body.actual_cost = cost
      body.currency = form.currency.trim().toUpperCase()
    }
    const res = await api.post<SO>(`/supplier-orders/${props.supplierOrder.id}/placed`, body)
    toast.success(`${res.number} recorded as placed.`)
    emit('placed', res)
  } catch (e) {
    capture(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <section class="space-y-4" aria-label="Placement sheet" data-testid="placement-sheet">
    <p class="text-sm text-slate-700">
      Place this order yourself on the supplier’s site, then record the supplier’s order number here. REA does not contact the supplier.
    </p>
    <ol class="space-y-2">
      <li v-for="(it, i) in supplierOrder.items" :key="it.line_id" class="rounded-md border border-slate-200 p-3">
        <div class="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
          <div class="min-w-0">
            <p class="text-sm font-medium break-words text-slate-800">{{ i + 1 }}. {{ it.title }}<span v-if="it.variant_title" class="font-normal text-slate-600"> ({{ it.variant_title }})</span></p>
            <p class="text-sm text-slate-700">Quantity <strong>{{ it.quantity }}</strong><span v-if="optionsText(it.options)"> · {{ optionsText(it.options) }}</span></p>
            <p v-if="it.sku" class="text-xs text-slate-500">SKU {{ it.sku }}</p>
            <p class="text-xs text-slate-500">
              Expected {{ it.expected_unit_cost ? `${formatMoney(it.expected_unit_cost, it.cost_currency)} each, ${formatMoney(lineCost(it.expected_unit_cost, it.quantity), it.cost_currency)}` : 'cost unknown' }}
            </p>
          </div>
          <div class="flex shrink-0 flex-wrap gap-2">
            <a v-if="safeLink(it.source_url)" :href="safeLink(it.source_url)" target="_blank" rel="noopener noreferrer nofollow"
              class="inline-flex items-center gap-1.5 rounded-md border border-slate-300 bg-white px-2.5 py-1 text-xs font-medium text-slate-700 shadow-xs hover:bg-slate-50">
              <ExternalLink class="h-3.5 w-3.5" aria-hidden="true" /> Open supplier page
            </a>
            <CopyButton v-if="it.source_url" :value="it.source_url" :label="`Copy the link of item ${i + 1}`" />
          </div>
        </div>
      </li>
    </ol>
    <p class="text-sm text-slate-700">
      Expected total: <strong>{{ supplierOrder.expected_cost_total ? formatMoney(supplierOrder.expected_cost_total, supplierOrder.cost_currency) : 'unknown' }}</strong>
    </p>
    <div>
      <h4 class="mb-1 text-xs font-semibold tracking-wide text-slate-500 uppercase">Ship to</h4>
      <AddressView :address="supplierOrder.ship_to" copy />
    </div>
    <div v-if="supplierOrder.note_to_supplier" class="flex items-start justify-between gap-2 rounded-md bg-slate-50 p-3">
      <p class="text-sm whitespace-pre-line text-slate-700"><span class="font-medium">Note to the supplier:</span> {{ supplierOrder.note_to_supplier }}</p>
      <CopyButton :value="supplierOrder.note_to_supplier" label="Copy the note to the supplier" />
    </div>

    <form v-if="auth.can('orders.write')" class="space-y-3 border-t border-slate-200 pt-4" data-testid="placed-form" @submit.prevent="submit">
      <h4 class="text-sm font-semibold text-slate-900">Record the placement</h4>
      <div class="grid gap-3 sm:grid-cols-3">
        <FormField v-slot="{ id }" label="Supplier order number" required :error="errors.supplier_order_number" class="sm:col-span-3">
          <input :id="id" v-model="form.supplier_order_number" class="input" maxlength="200" required />
        </FormField>
        <FormField v-slot="{ id }" label="Actual cost" :error="errors.actual_cost">
          <input :id="id" v-model="form.actual_cost" class="input" inputmode="decimal" placeholder="0.00" />
        </FormField>
        <FormField v-slot="{ id }" label="Currency" :error="errors.currency">
          <input :id="id" v-model="form.currency" class="input uppercase" maxlength="3" placeholder="EUR" />
        </FormField>
        <FormField v-slot="{ id }" label="Note" :error="errors.note" class="sm:col-span-3">
          <textarea :id="id" v-model="form.note" class="input" rows="2" maxlength="1000" />
        </FormField>
      </div>
      <div class="flex justify-end">
        <UiButton type="submit" variant="primary" :loading="saving" :disabled="!form.supplier_order_number.trim()">Record placement</UiButton>
      </div>
    </form>
  </section>
</template>
