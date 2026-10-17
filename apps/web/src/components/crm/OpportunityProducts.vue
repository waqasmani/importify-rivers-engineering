<script setup lang="ts">
// Product lines of an opportunity: list, add (product picker), edit and remove.
import { computed, reactive, ref } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { Pencil, Plus, Trash2 } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import UiButton from '@/components/ui/UiButton.vue'
import UiModal from '@/components/ui/UiModal.vue'
import FormField from '@/components/ui/FormField.vue'
import StateViews from '@/components/ui/StateViews.vue'
import EntityPicker from '@/components/record/EntityPicker.vue'
import { useAuth } from '@/stores/auth'
import { useSettings } from '@/stores/settings'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { formatMoney } from '@/lib/format'
import { lineTotals, toDecimal } from './crm'

type Line = Schemas['ListOpportunityProductsRow']

const props = defineProps<{ opportunityId: string; currency: string }>()
const auth = useAuth()
const toast = useToast()
const qc = useQueryClient()
const { confirm } = useConfirm()
const { label } = useSettings()
const { errors, capture, clear } = useFieldErrors()

const key = computed(() => ['opportunity-products', props.opportunityId])
const q = useQuery({
  queryKey: key,
  queryFn: ({ signal }) => api.get<Schemas['ListOpportunityProductsRowItems']>(`/opportunities/${props.opportunityId}/products`, undefined, signal),
})
const lines = computed(() => q.data.value?.items ?? [])
const totals = computed(() => lineTotals(lines.value, props.currency))
const canWrite = computed(() => auth.can('crm.write'))
const canPick = computed(() => auth.can('products.read'))

const modal = ref(false)
const editing = ref<Line | null>(null)
const saving = ref(false)
const form = reactive({ product_id: null as string | null, quantity: '1', unit_price: '', currency: '', notes: '' })

function open(line: Line | null) {
  clear()
  editing.value = line
  form.product_id = line?.product_id ?? null
  form.quantity = line?.quantity ?? '1'
  form.unit_price = line?.unit_price ?? ''
  form.currency = line?.currency ?? props.currency
  form.notes = line?.notes ?? ''
  modal.value = true
}

async function save() {
  const local: Record<string, string> = {}
  const quantity = toDecimal(form.quantity)
  const price = toDecimal(form.unit_price)
  if (!form.product_id) local.product_id = 'choose a product'
  if (!quantity) local.quantity = 'must be a number greater than zero'
  if (price === undefined) local.unit_price = 'must be a number such as 19.99'
  if (Object.keys(local).length) {
    errors.value = local
    return
  }
  saving.value = true
  try {
    const body: Schemas['OpportunityProductInput'] = { quantity: quantity!, unit_price: price ?? null, currency: form.currency.trim() ? form.currency.trim().toUpperCase() : null, notes: form.notes }
    await api.put(`/opportunities/${props.opportunityId}/products/${form.product_id}`, body)
    toast.success(editing.value ? 'Product line updated.' : 'Product line added.')
    modal.value = false
    await qc.invalidateQueries({ queryKey: key.value })
  } catch (e) {
    capture(e)
  } finally {
    saving.value = false
  }
}

async function remove(line: Line) {
  const ok = await confirm({ title: 'Remove product line?', message: `“${line.product_title}” will be removed from this ${label('opportunity').toLowerCase()}.`, confirmLabel: 'Remove', danger: true })
  if (!ok) return
  try {
    await api.del(`/opportunities/${props.opportunityId}/products/${line.product_id}`)
    toast.success('Product line removed.')
    await qc.invalidateQueries({ queryKey: key.value })
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
</script>

<template>
  <section class="card p-4" aria-labelledby="opp-products-title">
    <div class="mb-3 flex items-center justify-between gap-2">
      <h2 id="opp-products-title" class="text-sm font-semibold text-slate-800">{{ label('product', true) }}</h2>
      <UiButton v-if="canWrite && canPick" size="sm" @click="open(null)"><Plus class="h-4 w-4" /> Add {{ label('product').toLowerCase() }}</UiButton>
    </div>
    <StateViews v-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <p v-else-if="lines.length === 0" class="py-3 text-sm text-slate-500">No product lines yet.</p>
    <div v-else class="overflow-x-auto">
      <table class="min-w-full divide-y divide-slate-200 text-sm" data-testid="product-lines">
        <thead>
          <tr>
            <th class="table-th">{{ label('product') }}</th>
            <th class="table-th text-right">Qty</th>
            <th class="table-th hidden text-right sm:table-cell">Unit price</th>
            <th class="table-th text-right">Line total</th>
            <th v-if="canWrite" class="table-th"><span class="sr-only">Actions</span></th>
          </tr>
        </thead>
        <tbody class="divide-y divide-slate-100">
          <tr v-for="l in lines" :key="l.product_id">
            <td class="table-td">
              <RouterLink :to="`/products/${l.product_id}`" class="font-medium text-brand-700 hover:underline">{{ l.product_title }}</RouterLink>
              <span v-if="l.product_sku" class="block text-xs text-slate-500">SKU {{ l.product_sku }}</span>
              <span v-if="l.notes" class="block text-xs text-slate-500">{{ l.notes }}</span>
            </td>
            <td class="table-td text-right">{{ Number(l.quantity).toLocaleString() }}</td>
            <td class="table-td hidden text-right sm:table-cell">{{ formatMoney(l.unit_price, l.currency || currency) }}</td>
            <td class="table-td text-right">{{ l.unit_price ? formatMoney(String(Number(l.quantity) * Number(l.unit_price)), l.currency || currency) : '—' }}</td>
            <td v-if="canWrite" class="table-td whitespace-nowrap text-right">
              <button type="button" class="rounded p-1 text-slate-500 hover:bg-slate-100" :aria-label="`Edit line ${l.product_title}`" @click="open(l)"><Pencil class="h-4 w-4" /></button>
              <button type="button" class="rounded p-1 text-slate-500 hover:bg-red-50 hover:text-red-600" :aria-label="`Remove line ${l.product_title}`" @click="remove(l)"><Trash2 class="h-4 w-4" /></button>
            </td>
          </tr>
        </tbody>
        <tfoot v-if="totals.length">
          <tr v-for="t in totals" :key="t.currency">
            <td class="table-td font-medium" colspan="2">Total ({{ t.currency }})</td>
            <td class="hidden sm:table-cell" />
            <td class="table-td text-right font-semibold">{{ formatMoney(t.amount, t.currency) }}</td>
            <td v-if="canWrite" />
          </tr>
        </tfoot>
      </table>
    </div>

    <UiModal :open="modal" :title="editing ? `Edit ${editing.product_title}` : `Add ${label('product').toLowerCase()}`" @close="modal = false">
      <form id="opp-line-form" class="grid gap-4 sm:grid-cols-2" novalidate @submit.prevent="save">
        <FormField v-if="!editing" v-slot="{ id }" :label="label('product')" required :error="errors.product_id" class="sm:col-span-2">
          <EntityPicker v-model="form.product_id" type="product" :input-id="id" :invalid="!!errors.product_id" :exclude="lines.map((l) => l.product_id)" />
        </FormField>
        <FormField v-slot="{ id }" label="Quantity" required :error="errors.quantity">
          <input :id="id" v-model="form.quantity" class="input" inputmode="decimal" />
        </FormField>
        <FormField v-slot="{ id }" label="Unit price" :error="errors.unit_price">
          <input :id="id" v-model="form.unit_price" class="input" inputmode="decimal" placeholder="0.00" />
        </FormField>
        <FormField v-slot="{ id }" label="Currency" :error="errors.currency">
          <input :id="id" v-model="form.currency" class="input uppercase" maxlength="3" />
        </FormField>
        <FormField v-slot="{ id }" label="Notes" :error="errors.notes" class="sm:col-span-2">
          <textarea :id="id" v-model="form.notes" rows="2" class="input" maxlength="2000" />
        </FormField>
      </form>
      <template #footer>
        <UiButton variant="ghost" @click="modal = false">Cancel</UiButton>
        <UiButton type="submit" form="opp-line-form" variant="primary" :loading="saving">{{ editing ? 'Save line' : 'Add line' }}</UiButton>
      </template>
    </UiModal>
  </section>
</template>
