<script setup lang="ts">
// Enter a customer order by hand (for example a phone or e-mail order). REA prepares the draft
// supplier orders from it exactly as for orders received through an integration key.
import { computed, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useQueryClient } from '@tanstack/vue-query'
import { Plus, Trash2 } from 'lucide-vue-next'
import { api, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import EntityPicker from '@/components/record/EntityPicker.vue'
import AddressFields from '@/components/orders/AddressFields.vue'
import { useSettings } from '@/stores/settings'
import { useToast } from '@/composables/useToast'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { blankAddress, type Address } from '@/components/orders/order-utils'

interface LineForm {
  key: number
  product_id: string | null
  product_label: string
  sku: string
  title: string
  quantity: number
  unit_price: string
}

const router = useRouter()
const qc = useQueryClient()
const toast = useToast()
const { settings } = useSettings()
const { errors, capture } = useFieldErrors()
const saving = ref(false)
let seq = 0
const newLine = (): LineForm => ({ key: ++seq, product_id: null, product_label: '', sku: '', title: '', quantity: 1, unit_price: '' })

const form = reactive({
  external_id: '',
  currency: '',
  customer: { name: '', email: '', phone: '' },
  shipping_address: blankAddress() as Address,
  total: '',
  customer_note: '',
  lines: [newLine()] as LineForm[],
})
watch(
  () => settings.value?.imports.default_currency,
  (c) => {
    if (c && !form.currency) form.currency = c
  },
  { immediate: true },
)

const lineError = (i: number, f: string) => errors.value[`lines.${i}.${f}`]
const canSubmit = computed(() => form.lines.length > 0 && form.lines.every((l) => l.product_id || l.sku.trim() || l.title.trim()))

async function submit() {
  saving.value = true
  try {
    const body: Schemas['OrderInput'] = {
      external_id: form.external_id.trim(),
      currency: form.currency.trim().toUpperCase(),
      customer: { ...form.customer },
      shipping_address: { ...form.shipping_address },
      customer_note: form.customer_note,
      lines: form.lines.map((l) => {
        const line: Schemas['OrderLineInput'] = { quantity: Number(l.quantity), sku: l.sku.trim(), title: l.title.trim() || l.product_label }
        if (l.product_id) line.product_id = l.product_id
        if (l.unit_price.trim()) line.unit_price = l.unit_price.trim()
        return line
      }),
    }
    if (form.total.trim()) body.total = form.total.trim()
    const res = await api.post<Schemas['OrderDetail']>('/orders', body)
    toast.success(`Order ${res.number} saved; draft supplier orders prepared.`)
    await qc.invalidateQueries({ queryKey: ['orders'] })
    await router.push(`/orders/${res.id}`)
  } catch (e) {
    capture(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div>
    <PageHeader title="New order" subtitle="Enter a customer order by hand. Supplier orders are prepared as drafts; nothing is handed off until a person confirms each one." back="/orders" />
    <form class="space-y-6" novalidate data-testid="new-order-form" @submit.prevent="submit">
      <section class="card space-y-3 p-4" aria-labelledby="no-order">
        <h2 id="no-order" class="text-sm font-semibold text-slate-900">Order</h2>
        <div class="grid gap-3 sm:grid-cols-3">
          <FormField v-slot="{ id, describedby }" label="External order ID" :error="errors.external_id" hint="Optional. Entering the same ID twice returns the first order.">
            <input :id="id" v-model="form.external_id" class="input" maxlength="200" :aria-describedby="describedby" />
          </FormField>
          <FormField v-slot="{ id }" label="Currency" required :error="errors.currency">
            <input :id="id" v-model="form.currency" class="input uppercase" maxlength="3" required placeholder="EUR" />
          </FormField>
          <FormField v-slot="{ id }" label="Total paid by the customer" :error="errors.total">
            <input :id="id" v-model="form.total" class="input" inputmode="decimal" placeholder="0.00" />
          </FormField>
        </div>
      </section>

      <section class="card space-y-3 p-4" aria-labelledby="no-customer">
        <h2 id="no-customer" class="text-sm font-semibold text-slate-900">Customer</h2>
        <p class="text-xs text-slate-500">Personal data: visible only to people with orders.read, never sent to AI providers, and erasable once the order is done.</p>
        <div class="grid gap-3 sm:grid-cols-3">
          <FormField v-slot="{ id }" label="Name" :error="errors['customer.name']">
            <input :id="id" v-model="form.customer.name" class="input" maxlength="200" autocomplete="off" />
          </FormField>
          <FormField v-slot="{ id }" label="E-mail" :error="errors['customer.email']">
            <input :id="id" v-model="form.customer.email" type="email" class="input" maxlength="320" autocomplete="off" />
          </FormField>
          <FormField v-slot="{ id }" label="Phone" :error="errors['customer.phone']">
            <input :id="id" v-model="form.customer.phone" type="tel" class="input" maxlength="60" autocomplete="off" />
          </FormField>
        </div>
        <h3 class="pt-2 text-xs font-semibold tracking-wide text-slate-500 uppercase">Shipping address</h3>
        <AddressFields v-model="form.shipping_address" :errors="errors" prefix="shipping_address." id-prefix="new-order-address" />
      </section>

      <section class="card space-y-3 p-4" aria-labelledby="no-lines">
        <div class="flex items-center justify-between gap-2">
          <h2 id="no-lines" class="text-sm font-semibold text-slate-900">Lines</h2>
          <UiButton size="sm" :disabled="form.lines.length >= 200" @click="form.lines.push(newLine())"><Plus class="h-4 w-4" aria-hidden="true" /> Add line</UiButton>
        </div>
        <p v-if="errors.lines" class="text-xs text-red-600" role="alert">{{ errors.lines }}</p>
        <fieldset v-for="(l, i) in form.lines" :key="l.key" class="grid gap-3 rounded-md border border-slate-200 p-3 sm:grid-cols-6">
          <legend class="px-1 text-xs font-medium text-slate-500">Line {{ i + 1 }}</legend>
          <FormField v-slot="{ id }" label="Product" :error="lineError(i, 'product_id')" class="sm:col-span-3">
            <EntityPicker v-model="l.product_id" type="product" :input-id="id" placeholder="Search products" @select="(o) => (l.product_label = o?.label ?? '')" />
          </FormField>
          <FormField v-slot="{ id }" label="SKU" :error="lineError(i, 'sku')" hint="Used when no product is chosen: variant SKUs first, then product SKUs." class="sm:col-span-3">
            <input :id="id" v-model="l.sku" class="input" maxlength="200" :data-testid="`line-${i}-sku`" />
          </FormField>
          <FormField v-slot="{ id }" label="Title as sold" :error="lineError(i, 'title')" class="sm:col-span-3">
            <input :id="id" v-model="l.title" class="input" maxlength="500" />
          </FormField>
          <FormField v-slot="{ id }" label="Quantity" required :error="lineError(i, 'quantity')">
            <input :id="id" v-model.number="l.quantity" type="number" min="1" max="10000" class="input" required />
          </FormField>
          <FormField v-slot="{ id }" label="Unit price" :error="lineError(i, 'unit_price')">
            <input :id="id" v-model="l.unit_price" class="input" inputmode="decimal" placeholder="0.00" />
          </FormField>
          <div class="flex items-end">
            <UiButton v-if="form.lines.length > 1" variant="ghost" size="sm" :aria-label="`Remove line ${i + 1}`" @click="form.lines.splice(i, 1)"><Trash2 class="h-4 w-4" aria-hidden="true" /> Remove</UiButton>
          </div>
        </fieldset>
      </section>

      <section class="card space-y-3 p-4" aria-labelledby="no-note">
        <h2 id="no-note" class="text-sm font-semibold text-slate-900">Customer note</h2>
        <FormField v-slot="{ id }" label="Note" :error="errors.customer_note">
          <textarea :id="id" v-model="form.customer_note" class="input" rows="3" maxlength="2000" />
        </FormField>
      </section>

      <div class="flex justify-end gap-2">
        <RouterLink to="/orders" class="rounded-md px-3.5 py-2 text-sm font-medium text-slate-600 hover:bg-slate-100">Cancel</RouterLink>
        <UiButton type="submit" variant="primary" :loading="saving" :disabled="!canSubmit">Save order</UiButton>
      </div>
    </form>
  </div>
</template>
