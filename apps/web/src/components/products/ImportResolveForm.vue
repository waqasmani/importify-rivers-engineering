<script setup lang="ts">
// Decides what happens to an import REA parked as a possible duplicate.
import { computed, ref, useId, watch } from 'vue'
import { api, type Schemas } from '@/api/client'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import EntityPicker from '@/components/record/EntityPicker.vue'
import DuplicateList from './DuplicateList.vue'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { useToast } from '@/composables/useToast'
import type { DupSummary } from './product-utils'

const props = defineProps<{ importId: string; candidates: DupSummary[]; defaultTarget?: string | null }>()
const emit = defineEmits<{ resolved: [imp: Schemas['ProductImport']] }>()
const toast = useToast()
const { errors, capture, clear } = useFieldErrors()
const name = useId()

const decision = ref<'update_existing' | 'create_new' | 'skip'>(props.candidates.length ? 'update_existing' : 'create_new')
const target = ref<string | null>(props.defaultTarget ?? props.candidates[0]?.id ?? null)
const other = ref<string | null>(null)
const saving = ref(false)

watch(other, (id) => {
  if (id) target.value = id
  else if (!props.candidates.some((c) => c.id === target.value)) target.value = props.candidates[0]?.id ?? null
})
watch(target, (id) => {
  if (id && props.candidates.some((c) => c.id === id)) other.value = null
})

const label = computed(() => ({ update_existing: 'Update the selected product', create_new: 'Create a new draft product', skip: 'Skip this import' })[decision.value])

async function submit() {
  clear()
  saving.value = true
  try {
    const body: Schemas['ResolveInput'] = { decision: decision.value, target_product_id: decision.value === 'update_existing' ? target.value : null }
    const res = await api.post<Schemas['ProductImport']>(`/imports/${props.importId}/resolve`, body)
    toast.success(decision.value === 'skip' ? 'Import skipped.' : decision.value === 'create_new' ? 'Draft product created.' : 'Existing product updated.')
    emit('resolved', res)
  } catch (e) {
    capture(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <form class="space-y-4" data-testid="resolve-form" @submit.prevent="submit">
    <fieldset class="space-y-2">
      <legend class="label">Decision</legend>
      <label class="flex items-start gap-2 text-sm">
        <input v-model="decision" type="radio" :name="`${name}-decision`" value="update_existing" class="mt-0.5" />
        <span><span class="font-medium text-slate-800">Update an existing product</span><span class="block text-xs text-slate-500">Refreshes price, availability, specifications, images and variants. Curated fields such as title, category and supplier are kept.</span></span>
      </label>
      <label class="flex items-start gap-2 text-sm">
        <input v-model="decision" type="radio" :name="`${name}-decision`" value="create_new" class="mt-0.5" />
        <span><span class="font-medium text-slate-800">Create a new product anyway</span><span class="block text-xs text-slate-500">Stored as a draft that still needs review.</span></span>
      </label>
      <label class="flex items-start gap-2 text-sm">
        <input v-model="decision" type="radio" :name="`${name}-decision`" value="skip" class="mt-0.5" />
        <span><span class="font-medium text-slate-800">Skip</span><span class="block text-xs text-slate-500">Nothing is stored; the import is kept as skipped for the record.</span></span>
      </label>
      <p v-if="errors.decision" class="text-xs text-red-600" role="alert">{{ errors.decision }}</p>
    </fieldset>

    <div v-if="decision === 'update_existing'" class="space-y-3">
      <DuplicateList v-model="target" :candidates="candidates" selectable legend="Product to update" />
      <FormField v-slot="{ id }" label="Or choose another product" :error="errors.target_product_id">
        <EntityPicker v-model="other" type="product" :input-id="id" placeholder="Search products" :invalid="!!errors.target_product_id" />
      </FormField>
    </div>

    <UiButton type="submit" variant="primary" :loading="saving" :disabled="decision === 'update_existing' && !target">{{ label }}</UiButton>
  </form>
</template>
