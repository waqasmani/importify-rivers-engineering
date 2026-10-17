<script setup lang="ts">
// Map a customer order line to an REA product (and variant) by hand and set its quantity. Only
// lines that are not in a confirmed supplier order can change; the drafts are rebuilt.
import { computed, ref, watch } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { api, type Schemas } from '@/api/client'
import UiModal from '@/components/ui/UiModal.vue'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import EntityPicker from '@/components/record/EntityPicker.vue'
import { useToast } from '@/composables/useToast'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { optionsText } from './order-utils'

type Line = Schemas['OrderLineView']

const props = defineProps<{ open: boolean; orderId: string; line: Line }>()
const emit = defineEmits<{ close: []; saved: [order: Schemas['OrderDetail']] }>()
const toast = useToast()
const { errors, capture, clear } = useFieldErrors()
const saving = ref(false)
const productId = ref<string | null>(null)
const variantId = ref<string>('')
const quantity = ref(1)

watch(
  () => props.open,
  (open) => {
    if (!open) return
    clear()
    productId.value = props.line.product_id
    variantId.value = props.line.variant_id ?? ''
    quantity.value = props.line.quantity
  },
  { immediate: true },
)

const product = useQuery({
  queryKey: computed(() => ['product', productId.value]),
  queryFn: ({ signal }) => api.get<Schemas['ProductDetail']>(`/products/${productId.value}`, undefined, signal),
  enabled: computed(() => props.open && !!productId.value),
})
const variants = computed(() => product.data.value?.variants ?? [])
watch(productId, (id, prev) => {
  if (prev !== undefined && id !== prev) variantId.value = ''
})

async function submit() {
  if (!productId.value) return
  saving.value = true
  try {
    const body: Schemas['OrderLineUpdateInput'] = { product_id: productId.value, variant_id: variantId.value || null, quantity: Number(quantity.value) }
    const res = await api.put<Schemas['OrderDetail']>(`/orders/${props.orderId}/lines/${props.line.id}`, body)
    toast.success('Line updated; the draft supplier orders were prepared again.')
    emit('saved', res)
  } catch (e) {
    capture(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <UiModal :open="open" :title="`Map line ${line.position + 1}`" @close="emit('close')">
    <form id="line-map-form" class="space-y-4" @submit.prevent="submit">
      <p class="text-sm text-slate-700">Sold as <strong>{{ line.title || line.sku || 'untitled' }}</strong><span v-if="line.sku"> (SKU {{ line.sku }})</span>.</p>
      <FormField v-slot="{ id }" label="Product" required :error="errors.product_id">
        <EntityPicker v-model="productId" type="product" :input-id="id" :initial-label="line.sourcing?.product_title || null" placeholder="Search products" />
      </FormField>
      <FormField v-if="variants.length" v-slot="{ id }" label="Variant" :error="errors.variant_id" hint="Choose the variant the customer bought.">
        <select :id="id" v-model="variantId" class="input">
          <option value="">No variant</option>
          <option v-for="v in variants" :key="v.id" :value="v.id">{{ [v.title, v.sku, optionsText(v.options as Record<string, string>)].filter(Boolean).join(' · ') }}</option>
        </select>
      </FormField>
      <FormField v-slot="{ id }" label="Quantity" required :error="errors.quantity">
        <input :id="id" v-model.number="quantity" type="number" min="1" max="10000" class="input" required />
      </FormField>
    </form>
    <template #footer>
      <UiButton variant="ghost" @click="emit('close')">Cancel</UiButton>
      <UiButton type="submit" form="line-map-form" variant="primary" :loading="saving" :disabled="!productId">Save line</UiButton>
    </template>
  </UiModal>
</template>
