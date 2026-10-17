<script setup lang="ts">
// Edit the ship-to address and the note of a draft supplier order. Any change gives the draft a
// new content hash, so a confirmation prepared before the edit is refused.
import { reactive, ref, watch } from 'vue'
import { api, type Schemas } from '@/api/client'
import UiModal from '@/components/ui/UiModal.vue'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import AddressFields from './AddressFields.vue'
import { useToast } from '@/composables/useToast'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { blankAddress, type Address } from './order-utils'

type SO = Schemas['SupplierOrderView']

const props = defineProps<{ open: boolean; supplierOrder: SO }>()
const emit = defineEmits<{ close: []; saved: [so: SO] }>()
const toast = useToast()
const { errors, capture, clear } = useFieldErrors()
const saving = ref(false)
const form = reactive<{ ship_to: Address; note_to_supplier: string }>({ ship_to: blankAddress(), note_to_supplier: '' })
watch(
  () => props.open,
  (open) => {
    if (!open) return
    clear()
    form.ship_to = { ...blankAddress(), ...props.supplierOrder.ship_to }
    form.note_to_supplier = props.supplierOrder.note_to_supplier
  },
  { immediate: true },
)

async function submit() {
  saving.value = true
  try {
    const body: Schemas['SupplierOrderUpdateInput'] = { ship_to: form.ship_to, note_to_supplier: form.note_to_supplier }
    const res = await api.put<SO>(`/supplier-orders/${props.supplierOrder.id}`, body)
    toast.success(`${res.number} updated.`)
    emit('saved', res)
  } catch (e) {
    capture(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <UiModal :open="open" :title="`Ship-to of ${supplierOrder.number}`" size="lg" @close="emit('close')">
    <form id="ship-to-form" class="space-y-4" @submit.prevent="submit">
      <AddressFields v-model="form.ship_to" :errors="errors" prefix="ship_to." id-prefix="ship-to" />
      <FormField v-slot="{ id }" label="Note to the supplier" :error="errors.note_to_supplier">
        <textarea :id="id" v-model="form.note_to_supplier" class="input" rows="3" maxlength="2000" />
      </FormField>
    </form>
    <template #footer>
      <UiButton variant="ghost" @click="emit('close')">Cancel</UiButton>
      <UiButton type="submit" form="ship-to-form" variant="primary" :loading="saving">Save draft</UiButton>
    </template>
  </UiModal>
</template>
