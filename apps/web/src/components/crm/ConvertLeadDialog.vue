<script setup lang="ts">
// Converts a lead into an opportunity (POST /leads/{id}/convert, ConvertLeadInput).
import { reactive, ref, watch } from 'vue'
import { api, type Schemas } from '@/api/client'
import UiModal from '@/components/ui/UiModal.vue'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import EntityPicker from '@/components/record/EntityPicker.vue'
import StageSelect from './StageSelect.vue'
import { useSettings } from '@/stores/settings'
import { useToast } from '@/composables/useToast'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { toDecimal } from './crm'

const props = defineProps<{ open: boolean; lead: Schemas['LeadView'] }>()
const emit = defineEmits<{ close: []; converted: [doc: Schemas['ConvertLeadDoc']] }>()
const { label } = useSettings()
const toast = useToast()
const { errors, capture, clear } = useFieldErrors()
const busy = ref(false)
const form = reactive({ opportunity_name: '', amount: '', stage_id: null as string | null, supplier_id: null as string | null })

watch(
  () => props.open,
  (o) => {
    if (!o) return
    clear()
    form.opportunity_name = props.lead.title
    form.amount = props.lead.estimated_value ?? ''
    form.stage_id = null
    form.supplier_id = null
  },
  { immediate: true },
)

async function submit() {
  const amount = toDecimal(form.amount)
  if (amount === undefined) {
    errors.value = { amount: 'must be a number such as 1250.00' }
    return
  }
  busy.value = true
  try {
    const body: Schemas['ConvertLeadInput'] = { opportunity_name: form.opportunity_name, amount, stage_id: form.stage_id, supplier_id: form.supplier_id }
    const doc = await api.post<Schemas['ConvertLeadDoc']>(`/leads/${props.lead.id}/convert`, body)
    toast.success(`Converted to ${label('opportunity').toLowerCase()} “${doc.opportunity.name}”.`)
    emit('converted', doc)
    emit('close')
  } catch (e) {
    capture(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <UiModal :open="open" :title="`Convert to ${label('opportunity').toLowerCase()}`" @close="emit('close')">
    <form id="convert-form" class="grid gap-4 sm:grid-cols-2" novalidate @submit.prevent="submit">
      <p class="text-sm text-slate-600 sm:col-span-2">
        A new {{ label('opportunity').toLowerCase() }} is created with this {{ label('lead').toLowerCase() }}'s {{ label('company').toLowerCase() }}, {{ label('contact').toLowerCase() }}, owner, currency and description.
        The {{ label('lead').toLowerCase() }} is marked converted and moved to its pipeline's won stage.
      </p>
      <FormField v-slot="{ id }" :label="`${label('opportunity')} name`" :error="errors.opportunity_name || errors.name" class="sm:col-span-2" hint="Empty uses the lead title.">
        <input :id="id" v-model="form.opportunity_name" class="input" maxlength="200" />
      </FormField>
      <FormField v-slot="{ id }" :label="`Amount (${lead.currency})`" :error="errors.amount">
        <input :id="id" v-model="form.amount" class="input" inputmode="decimal" placeholder="0.00" />
      </FormField>
      <FormField v-slot="{ id }" label="Stage" :error="errors.stage_id || errors.pipeline_stage_id">
        <StageSelect v-model="form.stage_id" entity-type="opportunity" :input-id="id" none-label="First stage of the default pipeline" />
      </FormField>
      <FormField v-slot="{ id }" :label="label('supplier')" :error="errors.supplier_id" class="sm:col-span-2">
        <EntityPicker v-model="form.supplier_id" type="supplier" :input-id="id" :invalid="!!errors.supplier_id" />
      </FormField>
    </form>
    <template #footer>
      <UiButton variant="ghost" @click="emit('close')">Cancel</UiButton>
      <UiButton type="submit" form="convert-form" variant="primary" :loading="busy">Convert</UiButton>
    </template>
  </UiModal>
</template>
