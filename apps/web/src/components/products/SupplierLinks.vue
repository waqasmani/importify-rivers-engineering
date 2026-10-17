<script setup lang="ts">
// Suppliers that offer this product, with their SKU, cost, MOQ and lead time.
import { computed, reactive, ref } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { Pencil, Plus, Star, Trash2 } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiModal from '@/components/ui/UiModal.vue'
import FormField from '@/components/ui/FormField.vue'
import StateViews from '@/components/ui/StateViews.vue'
import EntityPicker from '@/components/record/EntityPicker.vue'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { useAuth } from '@/stores/auth'
import { useSettings } from '@/stores/settings'
import { formatMoney } from '@/lib/format'

type Link = Schemas['ListProductSuppliersRow']

const props = withDefaults(defineProps<{ productId: string; defaultCurrency?: string | null; canEdit?: boolean }>(), { defaultCurrency: null, canEdit: false })
const emit = defineEmits<{ changed: [] }>()
const auth = useAuth()
const toast = useToast()
const { confirm } = useConfirm()
const { label } = useSettings()
const { errors, capture, clear } = useFieldErrors()

const q = useQuery({
  queryKey: computed(() => ['product-suppliers', props.productId]),
  queryFn: ({ signal }) => api.get<Schemas['ListProductSuppliersRowItems']>(`/products/${props.productId}/suppliers`, undefined, signal),
})
const links = computed(() => q.data.value?.items ?? [])
const canLink = computed(() => props.canEdit && auth.can('crm.read'))

const modal = ref(false)
const editing = ref<Link | null>(null)
const saving = ref(false)
const form = reactive({ supplier_id: null as string | null, supplier_sku: '', cost: '' as string | number, currency: '', moq: '' as string | number, lead_time_days: '' as string | number, is_preferred: false, notes: '' })

function open(l: Link | null) {
  clear()
  editing.value = l
  form.supplier_id = l?.supplier_id ?? null
  form.supplier_sku = l?.supplier_sku ?? ''
  form.cost = l?.cost ?? ''
  form.currency = l?.currency ?? props.defaultCurrency ?? ''
  form.moq = l?.moq ?? ''
  form.lead_time_days = l?.lead_time_days ?? ''
  form.is_preferred = l?.is_preferred ?? links.value.length === 0
  form.notes = l?.notes ?? ''
  modal.value = true
}

const int = (v: string | number) => (String(v).trim() === '' ? null : Math.trunc(Number(v)))

async function save() {
  if (!form.supplier_id) {
    errors.value = { supplier_id: 'Choose a supplier.' }
    return
  }
  saving.value = true
  clear()
  const cost = String(form.cost ?? '').trim() || null
  const body: Schemas['SupplierProductInput'] = {
    supplier_sku: form.supplier_sku,
    cost,
    currency: form.currency.trim() ? form.currency.trim().toUpperCase() : null,
    moq: int(form.moq),
    lead_time_days: int(form.lead_time_days),
    is_preferred: form.is_preferred,
    notes: form.notes,
  }
  try {
    await api.put<Schemas['SupplierProduct']>(`/suppliers/${form.supplier_id}/products/${props.productId}`, body)
    toast.success(editing.value ? 'Supplier link saved.' : `${label('supplier')} linked.`)
    modal.value = false
    await q.refetch()
    emit('changed')
  } catch (e) {
    capture(e)
  } finally {
    saving.value = false
  }
}

async function remove(l: Link) {
  const ok = await confirm({ title: `Remove ${label('supplier').toLowerCase()}`, message: `Remove ${l.supplier_name} from this product? The ${label('supplier').toLowerCase()} record itself is kept.`, confirmLabel: 'Remove', danger: true })
  if (!ok) return
  try {
    await api.del(`/suppliers/${l.supplier_id}/products/${props.productId}`)
    toast.success('Supplier link removed.')
    await q.refetch()
    emit('changed')
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
</script>

<template>
  <section class="card p-4" aria-labelledby="suppliers-heading" data-testid="supplier-links">
    <div class="mb-3 flex items-center justify-between gap-2">
      <h2 id="suppliers-heading" class="text-sm font-semibold text-slate-800">{{ label('supplier', true) }}</h2>
      <UiButton v-if="canLink" size="sm" @click="open(null)"><Plus class="h-4 w-4" /> Link {{ label('supplier').toLowerCase() }}</UiButton>
    </div>
    <StateViews v-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <p v-else-if="links.length === 0" class="text-sm text-slate-500">No {{ label('supplier', true).toLowerCase() }} linked.</p>
    <ul v-else class="divide-y divide-slate-100">
      <li v-for="l in links" :key="l.supplier_id" class="flex flex-wrap items-start gap-3 py-2" :data-testid="`supplier-link-${l.supplier_id}`">
        <div class="min-w-0 flex-1">
          <div class="flex flex-wrap items-center gap-2">
            <RouterLink v-if="auth.can('crm.read')" :to="`/suppliers/${l.supplier_id}`" class="font-medium text-brand-700 hover:underline">{{ l.supplier_name }}</RouterLink>
            <span v-else class="font-medium text-slate-800">{{ l.supplier_name }}</span>
            <UiBadge v-if="l.is_preferred" tone="blue"><Star class="h-3 w-3" aria-hidden="true" /> Preferred</UiBadge>
            <span v-if="l.supplier_country" class="text-xs text-slate-500">{{ l.supplier_country }}</span>
          </div>
          <p class="mt-0.5 text-xs text-slate-600">
            <span v-if="l.supplier_sku">SKU {{ l.supplier_sku }} · </span>
            Cost {{ formatMoney(l.cost, l.currency) }}
            <span v-if="l.moq !== null"> · MOQ {{ l.moq }}</span>
            <span v-if="l.lead_time_days !== null"> · {{ l.lead_time_days }} days lead time</span>
          </p>
          <p v-if="l.notes" class="mt-0.5 text-xs whitespace-pre-wrap text-slate-500">{{ l.notes }}</p>
        </div>
        <div v-if="canEdit" class="flex gap-1">
          <button type="button" class="rounded p-1.5 text-slate-400 hover:bg-slate-100 hover:text-slate-700" :aria-label="`Edit link to ${l.supplier_name}`" @click="open(l)"><Pencil class="h-4 w-4" /></button>
          <button type="button" class="rounded p-1.5 text-slate-400 hover:bg-red-50 hover:text-red-600" :aria-label="`Remove ${l.supplier_name}`" @click="remove(l)"><Trash2 class="h-4 w-4" /></button>
        </div>
      </li>
    </ul>

    <UiModal :open="modal" :title="editing ? `Edit link to ${editing.supplier_name}` : `Link ${label('supplier').toLowerCase()}`" @close="modal = false">
      <form id="supplier-link-form" class="grid gap-4 sm:grid-cols-2" @submit.prevent="save">
        <FormField v-slot="{ id }" :label="label('supplier')" required :error="errors.supplier_id" class="sm:col-span-2">
          <EntityPicker v-model="form.supplier_id" type="supplier" :input-id="id" :initial-label="editing?.supplier_name ?? null" :disabled="!!editing" :exclude="editing ? [] : links.map((l) => l.supplier_id)" :invalid="!!errors.supplier_id" :placeholder="`Search ${label('supplier', true).toLowerCase()}`" />
        </FormField>
        <FormField v-slot="{ id }" :label="`${label('supplier')} SKU`" :error="errors.supplier_sku">
          <input :id="id" v-model="form.supplier_sku" class="input" maxlength="120" />
        </FormField>
        <div class="grid grid-cols-[1fr_6rem] gap-3">
          <FormField v-slot="{ id }" label="Cost" :error="errors.cost">
            <input :id="id" v-model="form.cost" type="number" min="0" step="any" inputmode="decimal" class="input" :class="{ 'input-invalid': errors.cost }" />
          </FormField>
          <FormField v-slot="{ id }" label="Currency" :error="errors.currency">
            <input :id="id" v-model="form.currency" class="input uppercase" :class="{ 'input-invalid': errors.currency }" maxlength="3" />
          </FormField>
        </div>
        <FormField v-slot="{ id }" label="Minimum order quantity" :error="errors.moq">
          <input :id="id" v-model="form.moq" type="number" min="0" step="1" class="input" :class="{ 'input-invalid': errors.moq }" />
        </FormField>
        <FormField v-slot="{ id }" label="Lead time (days)" :error="errors.lead_time_days">
          <input :id="id" v-model="form.lead_time_days" type="number" min="0" max="3650" step="1" class="input" :class="{ 'input-invalid': errors.lead_time_days }" />
        </FormField>
        <label class="flex items-center gap-2 text-sm text-slate-700 sm:col-span-2">
          <input v-model="form.is_preferred" type="checkbox" class="rounded border-slate-300" />
          Preferred {{ label('supplier').toLowerCase() }} for this product
        </label>
        <FormField v-slot="{ id }" label="Notes" :error="errors.notes" class="sm:col-span-2">
          <textarea :id="id" v-model="form.notes" rows="3" class="input" maxlength="5000" />
        </FormField>
      </form>
      <template #footer>
        <UiButton variant="ghost" @click="modal = false">Cancel</UiButton>
        <UiButton type="submit" form="supplier-link-form" variant="primary" :loading="saving">{{ editing ? 'Save link' : 'Link' }}</UiButton>
      </template>
    </UiModal>
  </section>
</template>
