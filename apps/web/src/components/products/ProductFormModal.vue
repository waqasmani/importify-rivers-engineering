<script setup lang="ts">
// Create or edit a product (ProductInput). Activation is not offered here: a product becomes active
// only through the review workflow. The selling price comes from the pricing rules unless "Manual
// price" locks a price entered here.
import { computed, reactive, ref, watch } from 'vue'
import { api, type Schemas } from '@/api/client'
import UiModal from '@/components/ui/UiModal.vue'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import EntityPicker from '@/components/record/EntityPicker.vue'
import CustomFieldsEditor from '@/components/record/CustomFieldsEditor.vue'
import KeyValueEditor from './KeyValueEditor.vue'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { useToast } from '@/composables/useToast'
import { useAuth } from '@/stores/auth'
import { useSettings } from '@/stores/settings'
import { titleCase } from '@/lib/format'
import { AVAILABILITIES, asRecord, mapFromRows, rowsFromMap, stringMap, validGtin, type KvRow } from './product-utils'

type Detail = Schemas['ProductDetail']

const props = defineProps<{ open: boolean; product?: Detail | null }>()
const emit = defineEmits<{ close: []; saved: [product: Detail] }>()
const auth = useAuth()
const toast = useToast()
const { label, settings } = useSettings()
const { errors, capture, clear } = useFieldErrors()
const saving = ref(false)

const form = reactive({
  title: '', source_url: '', brand: '', sku: '', model: '', mpn: '', gtin: '', category: '', availability: 'unknown', status: 'draft',
  price: '', price_currency: '', cost: '', cost_currency: '', description: '', internal_notes: '', price_locked: false, compare_at_price: '',
  supplier_id: null as string | null, supplier_contact_id: null as string | null, company_id: null as string | null,
  custom_fields: {} as Record<string, unknown>,
})
const specs = ref<KvRow[]>([])
const identifiers = ref<KvRow[]>([])

watch(
  () => props.open,
  (open) => {
    if (!open) return
    clear()
    const p = props.product
    const cur = settings.value?.imports.default_currency ?? ''
    form.title = p?.title ?? ''
    form.source_url = p?.source_url ?? ''
    form.brand = p?.brand ?? ''
    form.sku = p?.sku ?? ''
    form.model = p?.model ?? ''
    form.mpn = p?.mpn ?? ''
    form.gtin = p?.gtin ?? ''
    form.category = p?.category ?? ''
    form.availability = p?.availability ?? 'unknown'
    form.status = p?.status ?? 'draft'
    form.price = p?.price ?? ''
    form.price_currency = p?.price_currency ?? cur
    form.cost = p?.cost ?? ''
    form.cost_currency = p?.cost_currency ?? (p?.price_currency || cur)
    form.price_locked = p?.price_locked ?? false
    form.compare_at_price = p?.compare_at_price ?? ''
    form.description = p?.description ?? ''
    form.internal_notes = p?.internal_notes ?? ''
    form.supplier_id = p?.supplier_id ?? null
    form.supplier_contact_id = p?.supplier_contact_id ?? null
    form.company_id = p?.company_id ?? null
    form.custom_fields = { ...asRecord(p?.custom_fields) }
    specs.value = rowsFromMap(stringMap(p?.specifications))
    identifiers.value = rowsFromMap(stringMap(p?.identifiers))
  },
  { immediate: true },
)

const statuses = computed(() => {
  const s = ['draft', 'archived']
  if (props.product && (props.product.review_status === 'approved' || props.product.status === 'active')) s.splice(1, 0, 'active')
  return s
})
const gtinHint = computed(() => {
  const g = form.gtin.trim()
  if (!g) return 'Optional. Must have a valid check digit.'
  return validGtin(g) ? `Valid GTIN-${g.length}.` : 'This is not a valid GTIN-8, 12, 13 or 14; the server will reject it.'
})
const canPickCrm = computed(() => auth.can('crm.read'))
const costHint = computed(() => {
  const p = props.product
  if (p?.source_price && !p.cost_manual) return `Derived from the source price (${p.source_price} ${p.source_currency ?? ''}). Enter another value to set it by hand; clear it to derive it again.`
  if (p?.source_price) return 'Entered by hand. Clear it to derive the cost from the source price again.'
  return 'What you pay the supplier; the pricing rules turn it into the selling price.'
})
// Number inputs bind numbers; decimals are sent to the API as strings.
const blank = (s: unknown) => String(s ?? '').trim() || null

async function submit() {
  saving.value = true
  clear()
  const body: Schemas['ProductInput'] = {
    title: form.title,
    source_url: blank(form.source_url),
    brand: form.brand,
    sku: form.sku,
    model: form.model,
    mpn: form.mpn,
    gtin: blank(form.gtin),
    identifiers: mapFromRows(identifiers.value),
    description: form.description,
    specifications: mapFromRows(specs.value),
    // Without a manual price the rules own price and compare-at price; the stored values are sent back unchanged.
    price: form.price_locked ? blank(form.price) : (props.product?.price ?? null),
    price_currency: form.price_locked ? (blank(form.price_currency)?.toUpperCase() ?? null) : (props.product?.price_currency ?? null),
    price_locked: form.price_locked,
    compare_at_price: form.price_locked ? blank(form.compare_at_price) : (props.product?.compare_at_price ?? null),
    cost: blank(form.cost),
    cost_currency: blank(form.cost_currency)?.toUpperCase() ?? null,
    availability: form.availability,
    category: form.category,
    status: form.status,
    internal_notes: form.internal_notes,
    custom_fields: form.custom_fields,
    supplier_id: form.supplier_id,
    supplier_contact_id: form.supplier_contact_id,
    company_id: form.company_id,
  }
  // A currency only means something next to an amount.
  if (!body.price) body.price_currency = null
  if (!body.cost) body.cost_currency = null
  try {
    const saved = props.product ? await api.put<Detail>(`/products/${props.product.id}`, body) : await api.post<Detail>('/products', body)
    toast.success(props.product ? 'Product saved.' : 'Draft product created.')
    emit('saved', saved)
    emit('close')
  } catch (e) {
    capture(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <UiModal :open="open" :title="product ? `Edit ${label('product').toLowerCase()}` : `New ${label('product').toLowerCase()}`" size="xl" @close="emit('close')">
    <form id="product-form" class="space-y-6" data-testid="product-form" @submit.prevent="submit">
      <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <FormField v-slot="{ id }" label="Title" required :error="errors.title" class="sm:col-span-2 lg:col-span-3">
          <input :id="id" v-model="form.title" class="input" :class="{ 'input-invalid': errors.title }" maxlength="500" required />
        </FormField>
        <FormField v-slot="{ id }" label="Source URL" :error="errors.source_url" class="sm:col-span-2">
          <input :id="id" v-model="form.source_url" type="url" class="input" maxlength="2048" placeholder="https://" />
        </FormField>
        <FormField v-slot="{ id }" label="Category" :error="errors.category">
          <input :id="id" v-model="form.category" class="input" maxlength="300" />
        </FormField>
        <FormField v-slot="{ id }" label="Brand" :error="errors.brand">
          <input :id="id" v-model="form.brand" class="input" maxlength="200" />
        </FormField>
        <FormField v-slot="{ id }" label="Model" :error="errors.model">
          <input :id="id" v-model="form.model" class="input" maxlength="200" />
        </FormField>
        <FormField v-slot="{ id }" label="SKU" :error="errors.sku">
          <input :id="id" v-model="form.sku" class="input" maxlength="120" />
        </FormField>
        <FormField v-slot="{ id }" label="MPN" :error="errors.mpn">
          <input :id="id" v-model="form.mpn" class="input" maxlength="120" />
        </FormField>
        <FormField v-slot="{ id, describedby }" label="GTIN / EAN / UPC" :error="errors.gtin" :hint="gtinHint">
          <input :id="id" v-model="form.gtin" class="input" :class="{ 'input-invalid': errors.gtin }" inputmode="numeric" maxlength="14" :aria-describedby="describedby" />
        </FormField>
        <FormField v-slot="{ id }" label="Availability" :error="errors.availability">
          <select :id="id" v-model="form.availability" class="input">
            <option v-for="a in AVAILABILITIES" :key="a" :value="a">{{ titleCase(a) }}</option>
          </select>
        </FormField>
        <div class="grid grid-cols-[1fr_6rem] gap-3">
          <FormField v-slot="{ id, describedby }" label="Cost" :error="errors.cost" :hint="costHint">
            <input :id="id" v-model="form.cost" type="number" min="0" step="any" inputmode="decimal" class="input" :class="{ 'input-invalid': errors.cost }" :aria-describedby="describedby" />
          </FormField>
          <FormField v-slot="{ id }" label="Cost currency" :error="errors.cost_currency">
            <input :id="id" v-model="form.cost_currency" class="input uppercase" :class="{ 'input-invalid': errors.cost_currency }" maxlength="3" />
          </FormField>
        </div>
        <fieldset class="space-y-2 sm:col-span-2" data-testid="price-fields">
          <legend class="sr-only">Selling price</legend>
          <label class="flex items-center gap-2 text-sm text-slate-700">
            <input v-model="form.price_locked" type="checkbox" class="rounded border-slate-300" data-testid="price-lock" /> Manual price (pricing rules do not change it)
          </label>
          <div class="grid grid-cols-[1fr_6rem_1fr] gap-3">
            <FormField v-slot="{ id, describedby }" label="Price" :error="errors.price" :hint="form.price_locked ? undefined : 'Set by the pricing rules.'">
              <input :id="id" v-model="form.price" type="number" min="0" step="any" inputmode="decimal" class="input" :class="{ 'input-invalid': errors.price }" :disabled="!form.price_locked" :aria-describedby="describedby" />
            </FormField>
            <FormField v-slot="{ id }" label="Currency" :error="errors.price_currency">
              <input :id="id" v-model="form.price_currency" class="input uppercase" :class="{ 'input-invalid': errors.price_currency }" maxlength="3" :disabled="!form.price_locked" />
            </FormField>
            <FormField v-slot="{ id }" label="Compare-at price" :error="errors.compare_at_price">
              <input :id="id" v-model="form.compare_at_price" type="number" min="0" step="any" inputmode="decimal" class="input" :disabled="!form.price_locked" />
            </FormField>
          </div>
        </fieldset>
        <FormField v-if="product" v-slot="{ id, describedby }" label="Status" :error="errors.status" hint="Products become active only when approved in review.">
          <select :id="id" v-model="form.status" class="input" :aria-describedby="describedby">
            <option v-for="s in statuses" :key="s" :value="s">{{ titleCase(s) }}</option>
          </select>
        </FormField>
        <template v-if="canPickCrm">
          <FormField v-slot="{ id }" :label="label('supplier')" :error="errors.supplier_id">
            <EntityPicker v-model="form.supplier_id" type="supplier" :input-id="id" :initial-label="product?.supplier_name ?? null" :placeholder="`Search ${label('supplier', true).toLowerCase()}`" />
          </FormField>
          <FormField v-slot="{ id }" :label="`${label('supplier')} ${label('contact').toLowerCase()}`" :error="errors.supplier_contact_id">
            <EntityPicker v-model="form.supplier_contact_id" type="contact" :input-id="id" :placeholder="`Search ${label('contact', true).toLowerCase()}`" />
          </FormField>
          <FormField v-slot="{ id }" :label="label('company')" :error="errors.company_id">
            <EntityPicker v-model="form.company_id" type="company" :input-id="id" :placeholder="`Search ${label('company', true).toLowerCase()}`" />
          </FormField>
        </template>
        <FormField v-slot="{ id }" label="Description" :error="errors.description" class="sm:col-span-2 lg:col-span-3">
          <textarea :id="id" v-model="form.description" rows="5" class="input" maxlength="50000" />
        </FormField>
      </div>

      <div class="grid gap-6 lg:grid-cols-2">
        <KeyValueEditor v-model="specs" label="Specifications" key-label="Specification" add-label="Add specification" :error="errors.specifications" />
        <KeyValueEditor v-model="identifiers" label="Other identifiers" key-label="Identifier" add-label="Add identifier" :max="30" :error="errors.identifiers" />
      </div>

      <FormField v-slot="{ id }" label="Internal notes" :error="errors.internal_notes" hint="Visible to your team only.">
        <textarea :id="id" v-model="form.internal_notes" rows="3" class="input" maxlength="20000" />
      </FormField>

      <CustomFieldsEditor v-model="form.custom_fields" entity-type="product" :errors="errors" />
    </form>
    <template #footer>
      <UiButton variant="ghost" @click="emit('close')">Cancel</UiButton>
      <UiButton type="submit" form="product-form" variant="primary" :loading="saving">{{ product ? 'Save product' : 'Create draft product' }}</UiButton>
    </template>
  </UiModal>
</template>
