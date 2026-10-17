<script setup lang="ts">
// Products sourced from a supplier: link a product (cost, MOQ, lead time, preferred) and edit or
// remove links. Editing requires products.write.
import { computed, reactive, ref } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { Pencil, Plus, Trash2 } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import UiButton from '@/components/ui/UiButton.vue'
import UiModal from '@/components/ui/UiModal.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import FormField from '@/components/ui/FormField.vue'
import StateViews from '@/components/ui/StateViews.vue'
import EntityPicker from '@/components/record/EntityPicker.vue'
import { useAuth } from '@/stores/auth'
import { useSettings } from '@/stores/settings'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { formatMoney } from '@/lib/format'
import { toDecimal, toInt } from './crm'

type Link = Schemas['ListSupplierProductsRow']

const props = defineProps<{ supplierId: string; supplierName: string }>()
const auth = useAuth()
const toast = useToast()
const qc = useQueryClient()
const { confirm } = useConfirm()
const { label, settings } = useSettings()
const { errors, capture, clear } = useFieldErrors()

const key = computed(() => ['supplier-products', props.supplierId])
const q = useQuery({
  queryKey: key,
  queryFn: ({ signal }) => api.get<Schemas['ListSupplierProductsRowItems']>(`/suppliers/${props.supplierId}/products`, undefined, signal),
})
const links = computed(() => q.data.value?.items ?? [])
const canWrite = computed(() => auth.can('products.write'))

const modal = ref(false)
const editing = ref<Link | null>(null)
const saving = ref(false)
const form = reactive({ product_id: null as string | null, supplier_sku: '', cost: '', currency: '', moq: '', lead_time_days: '', is_preferred: false, notes: '' })

function open(link: Link | null) {
  clear()
  editing.value = link
  form.product_id = link?.product_id ?? null
  form.supplier_sku = link?.supplier_sku ?? ''
  form.cost = link?.cost ?? ''
  form.currency = link?.currency ?? settings.value?.imports.default_currency ?? ''
  form.moq = link?.moq === null || link?.moq === undefined ? '' : String(link.moq)
  form.lead_time_days = link?.lead_time_days === null || link?.lead_time_days === undefined ? '' : String(link.lead_time_days)
  form.is_preferred = link?.is_preferred ?? false
  form.notes = link?.notes ?? ''
  modal.value = true
}

async function save() {
  const local: Record<string, string> = {}
  const cost = toDecimal(form.cost)
  const moq = toInt(form.moq)
  const lead = toInt(form.lead_time_days)
  if (!form.product_id) local.product_id = 'choose a product'
  if (cost === undefined) local.cost = 'must be a number such as 4.25'
  if (moq === undefined) local.moq = 'must be a whole number'
  if (lead === undefined) local.lead_time_days = 'must be a whole number of days'
  if (Object.keys(local).length) {
    errors.value = local
    return
  }
  saving.value = true
  try {
    const currency = form.currency.trim().toUpperCase()
    const body: Schemas['SupplierProductInput'] = {
      supplier_sku: form.supplier_sku, cost: cost ?? null, currency: currency || null, moq: moq ?? null,
      lead_time_days: lead ?? null, is_preferred: form.is_preferred, notes: form.notes,
    }
    await api.put(`/suppliers/${props.supplierId}/products/${form.product_id}`, body)
    toast.success(editing.value ? 'Product link updated.' : `${label('product')} linked to ${props.supplierName}.`)
    modal.value = false
    await Promise.all([qc.invalidateQueries({ queryKey: key.value }), qc.invalidateQueries({ queryKey: ['supplier', props.supplierId] })])
  } catch (e) {
    capture(e)
  } finally {
    saving.value = false
  }
}

async function remove(link: Link) {
  const ok = await confirm({ title: 'Remove product link?', message: `“${link.product_title}” will no longer be listed for ${props.supplierName}. The product itself is kept.`, confirmLabel: 'Remove link', danger: true })
  if (!ok) return
  try {
    await api.del(`/suppliers/${props.supplierId}/products/${link.product_id}`)
    toast.success('Product link removed.')
    await qc.invalidateQueries({ queryKey: key.value })
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
</script>

<template>
  <section class="card p-4" aria-labelledby="supplier-products-title">
    <div class="mb-3 flex items-center justify-between gap-2">
      <h2 id="supplier-products-title" class="text-sm font-semibold text-slate-800">{{ label('product', true) }} <span v-if="q.data.value" class="font-normal text-slate-500">({{ links.length }})</span></h2>
      <UiButton v-if="canWrite && auth.can('products.read')" size="sm" @click="open(null)"><Plus class="h-4 w-4" /> Link {{ label('product').toLowerCase() }}</UiButton>
    </div>
    <StateViews v-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <p v-else-if="links.length === 0" class="py-3 text-sm text-slate-500">No {{ label('product', true).toLowerCase() }} linked yet.</p>
    <div v-else class="overflow-x-auto">
      <table class="min-w-full divide-y divide-slate-200 text-sm" data-testid="supplier-products">
        <thead>
          <tr>
            <th class="table-th">{{ label('product') }}</th>
            <th class="table-th hidden sm:table-cell">Supplier SKU</th>
            <th class="table-th text-right">Cost</th>
            <th class="table-th text-right">MOQ</th>
            <th class="table-th hidden text-right sm:table-cell">Lead time</th>
            <th v-if="canWrite" class="table-th"><span class="sr-only">Actions</span></th>
          </tr>
        </thead>
        <tbody class="divide-y divide-slate-100">
          <tr v-for="l in links" :key="l.product_id">
            <td class="table-td">
              <RouterLink :to="`/products/${l.product_id}`" class="font-medium text-brand-700 hover:underline">{{ l.product_title }}</RouterLink>
              <span class="mt-0.5 flex flex-wrap gap-1">
                <UiBadge :value="l.product_status" />
                <UiBadge v-if="l.is_preferred" tone="green">preferred</UiBadge>
              </span>
              <span v-if="l.notes" class="block text-xs text-slate-500">{{ l.notes }}</span>
            </td>
            <td class="table-td hidden sm:table-cell">{{ l.supplier_sku || '—' }}</td>
            <td class="table-td text-right">{{ formatMoney(l.cost, l.currency) }}</td>
            <td class="table-td text-right">{{ l.moq ?? '—' }}</td>
            <td class="table-td hidden text-right sm:table-cell">{{ l.lead_time_days !== null && l.lead_time_days !== undefined ? `${l.lead_time_days} d` : '—' }}</td>
            <td v-if="canWrite" class="table-td whitespace-nowrap text-right">
              <button type="button" class="rounded p-1 text-slate-500 hover:bg-slate-100" :aria-label="`Edit link to ${l.product_title}`" @click="open(l)"><Pencil class="h-4 w-4" /></button>
              <button type="button" class="rounded p-1 text-slate-500 hover:bg-red-50 hover:text-red-600" :aria-label="`Remove link to ${l.product_title}`" @click="remove(l)"><Trash2 class="h-4 w-4" /></button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <UiModal :open="modal" :title="editing ? `Edit link to ${editing.product_title}` : `Link a ${label('product').toLowerCase()}`" @close="modal = false">
      <form id="supplier-link-form" class="grid gap-4 sm:grid-cols-2" novalidate @submit.prevent="save">
        <FormField v-if="!editing" v-slot="{ id }" :label="label('product')" required :error="errors.product_id" class="sm:col-span-2">
          <EntityPicker v-model="form.product_id" type="product" :input-id="id" :invalid="!!errors.product_id" :exclude="links.map((l) => l.product_id)" />
        </FormField>
        <FormField v-slot="{ id }" label="Supplier SKU" :error="errors.supplier_sku">
          <input :id="id" v-model="form.supplier_sku" class="input" maxlength="120" />
        </FormField>
        <FormField v-slot="{ id }" label="MOQ" :error="errors.moq">
          <input :id="id" v-model="form.moq" class="input" inputmode="numeric" />
        </FormField>
        <FormField v-slot="{ id }" label="Cost" :error="errors.cost">
          <input :id="id" v-model="form.cost" class="input" inputmode="decimal" placeholder="0.00" />
        </FormField>
        <FormField v-slot="{ id }" label="Currency" :error="errors.currency" hint="Required when a cost is set.">
          <input :id="id" v-model="form.currency" class="input uppercase" maxlength="3" />
        </FormField>
        <FormField v-slot="{ id }" label="Lead time (days)" :error="errors.lead_time_days">
          <input :id="id" v-model="form.lead_time_days" class="input" inputmode="numeric" />
        </FormField>
        <div class="flex items-end pb-2">
          <label class="flex items-center gap-2 text-sm text-slate-700"><input v-model="form.is_preferred" type="checkbox" class="rounded border-slate-300" /> Preferred supplier for this product</label>
        </div>
        <FormField v-slot="{ id }" label="Notes" :error="errors.notes" class="sm:col-span-2">
          <textarea :id="id" v-model="form.notes" rows="2" class="input" maxlength="5000" />
        </FormField>
      </form>
      <template #footer>
        <UiButton variant="ghost" @click="modal = false">Cancel</UiButton>
        <UiButton type="submit" form="supplier-link-form" variant="primary" :loading="saving">{{ editing ? 'Save link' : 'Link product' }}</UiButton>
      </template>
    </UiModal>
  </section>
</template>
