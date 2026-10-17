<script setup lang="ts">
// Record or correct the tracking of a placed supplier order.
import { reactive, ref, watch } from 'vue'
import { api, type Schemas } from '@/api/client'
import UiModal from '@/components/ui/UiModal.vue'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import { useToast } from '@/composables/useToast'
import { useFieldErrors } from '@/composables/useFieldErrors'

type SO = Schemas['SupplierOrderView']

const props = defineProps<{ open: boolean; supplierOrder: SO }>()
const emit = defineEmits<{ close: []; saved: [so: SO] }>()
const toast = useToast()
const { errors, capture, clear } = useFieldErrors()
const saving = ref(false)
const form = reactive({ carrier: '', tracking_number: '', tracking_url: '' })
watch(
  () => props.open,
  (open) => {
    if (!open) return
    clear()
    const so = props.supplierOrder
    Object.assign(form, { carrier: so.carrier, tracking_number: so.tracking_number, tracking_url: so.tracking_url ?? '' })
  },
  { immediate: true },
)

async function submit() {
  saving.value = true
  try {
    const body: Schemas['ShipmentInput'] = { carrier: form.carrier, tracking_number: form.tracking_number.trim(), tracking_url: form.tracking_url.trim() }
    const res = await api.post<SO>(`/supplier-orders/${props.supplierOrder.id}/shipment`, body)
    toast.success(`Tracking recorded for ${res.number}.`)
    emit('saved', res)
  } catch (e) {
    capture(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <UiModal :open="open" :title="`Shipment of ${supplierOrder.number}`" @close="emit('close')">
    <form id="shipment-form" class="space-y-3" @submit.prevent="submit">
      <FormField v-slot="{ id }" label="Carrier" :error="errors.carrier">
        <input :id="id" v-model="form.carrier" class="input" maxlength="100" placeholder="DHL" />
      </FormField>
      <FormField v-slot="{ id }" label="Tracking number" required :error="errors.tracking_number">
        <input :id="id" v-model="form.tracking_number" class="input" maxlength="200" required />
      </FormField>
      <FormField v-slot="{ id, describedby }" label="Tracking link" :error="errors.tracking_url" hint="Optional. Must start with https://.">
        <input :id="id" v-model="form.tracking_url" type="url" class="input" maxlength="2048" placeholder="https://" :aria-describedby="describedby" />
      </FormField>
    </form>
    <template #footer>
      <UiButton variant="ghost" @click="emit('close')">Cancel</UiButton>
      <UiButton type="submit" form="shipment-form" variant="primary" :loading="saving" :disabled="!form.tracking_number.trim()">Save tracking</UiButton>
    </template>
  </UiModal>
</template>
