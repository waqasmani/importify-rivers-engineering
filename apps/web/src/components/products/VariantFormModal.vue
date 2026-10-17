<script setup lang="ts">
// Create or edit one product variant (VariantInput).
import { reactive, ref, watch } from 'vue'
import { api, type Schemas } from '@/api/client'
import UiModal from '@/components/ui/UiModal.vue'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import KeyValueEditor from './KeyValueEditor.vue'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { useToast } from '@/composables/useToast'
import { titleCase } from '@/lib/format'
import { AVAILABILITIES, mapFromRows, rowsFromMap, stringMap, type KvRow } from './product-utils'

type Variant = Schemas['ProductVariant']

const props = defineProps<{ open: boolean; productId: string; variant?: Variant | null; defaultCurrency?: string | null; nextPosition: number }>()
const emit = defineEmits<{ close: []; saved: [variant: Variant] }>()
const toast = useToast()
const { errors, capture, clear } = useFieldErrors()
const saving = ref(false)

const form = reactive({ title: '', sku: '', price: '', currency: '', availability: 'unknown', image_url: '', position: 0 })
const options = ref<KvRow[]>([])

watch(
  () => props.open,
  (open) => {
    if (!open) return
    clear()
    const v = props.variant
    form.title = v?.title ?? ''
    form.sku = v?.sku ?? ''
    form.price = v?.price ?? ''
    form.currency = v?.currency ?? props.defaultCurrency ?? ''
    form.availability = v?.availability ?? 'unknown'
    form.image_url = v?.image_url ?? ''
    form.position = v?.position ?? props.nextPosition
    options.value = rowsFromMap(stringMap(v?.options))
  },
  { immediate: true },
)

async function submit() {
  saving.value = true
  clear()
  // v-model on number inputs yields numbers; the API takes decimals as strings.
  const price = String(form.price ?? '').trim() || null
  const body: Schemas['VariantInput'] = {
    title: form.title,
    sku: form.sku,
    options: mapFromRows(options.value),
    price,
    currency: form.currency.trim() ? form.currency.trim().toUpperCase() : null,
    availability: form.availability,
    image_url: form.image_url.trim() || null,
    position: Number(form.position) || 0,
  }
  try {
    const saved = props.variant
      ? await api.put<Variant>(`/products/${props.productId}/variants/${props.variant.id}`, body)
      : await api.post<Variant>(`/products/${props.productId}/variants`, body)
    toast.success(props.variant ? 'Variant saved.' : 'Variant added.')
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
  <UiModal :open="open" :title="variant ? 'Edit variant' : 'Add variant'" size="lg" @close="emit('close')">
    <form id="variant-form" class="space-y-5" data-testid="variant-form" @submit.prevent="submit">
      <p class="text-sm text-slate-600">A variant needs at least a title, a SKU or one option.</p>
      <div class="grid gap-4 sm:grid-cols-2">
        <FormField v-slot="{ id }" label="Title" :error="errors.title">
          <input :id="id" v-model="form.title" class="input" :class="{ 'input-invalid': errors.title }" maxlength="300" placeholder="Black / Large" />
        </FormField>
        <FormField v-slot="{ id }" label="SKU" :error="errors.sku">
          <input :id="id" v-model="form.sku" class="input" maxlength="120" />
        </FormField>
        <div class="grid grid-cols-[1fr_6rem] gap-3">
          <FormField v-slot="{ id }" label="Price" :error="errors.price">
            <input :id="id" v-model="form.price" type="number" min="0" step="any" inputmode="decimal" class="input" :class="{ 'input-invalid': errors.price }" />
          </FormField>
          <FormField v-slot="{ id }" label="Currency" :error="errors.currency">
            <input :id="id" v-model="form.currency" class="input uppercase" :class="{ 'input-invalid': errors.currency }" maxlength="3" />
          </FormField>
        </div>
        <FormField v-slot="{ id }" label="Availability" :error="errors.availability">
          <select :id="id" v-model="form.availability" class="input">
            <option v-for="a in AVAILABILITIES" :key="a" :value="a">{{ titleCase(a) }}</option>
          </select>
        </FormField>
        <FormField v-slot="{ id }" label="Image URL" :error="errors.image_url">
          <input :id="id" v-model="form.image_url" type="url" class="input" :class="{ 'input-invalid': errors.image_url }" maxlength="2048" placeholder="https://" />
        </FormField>
        <FormField v-slot="{ id, describedby }" label="Position" :error="errors.position" hint="Lower numbers are listed first.">
          <input :id="id" v-model.number="form.position" type="number" min="0" step="1" class="input" :aria-describedby="describedby" />
        </FormField>
      </div>
      <KeyValueEditor v-model="options" label="Options" key-label="Option" value-label="Value" add-label="Add option" :max="10" :error="errors.options" />
    </form>
    <template #footer>
      <UiButton variant="ghost" @click="emit('close')">Cancel</UiButton>
      <UiButton type="submit" form="variant-form" variant="primary" :loading="saving">{{ variant ? 'Save variant' : 'Add variant' }}</UiButton>
    </template>
  </UiModal>
</template>
