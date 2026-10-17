<script setup lang="ts">
// Create or edit an opportunity (OpportunityInput). The stage decides open/won/lost on the server.
import { computed, reactive, ref, watch } from 'vue'
import { api, type Schemas } from '@/api/client'
import UiModal from '@/components/ui/UiModal.vue'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import EntityPicker from '@/components/record/EntityPicker.vue'
import CustomFieldsEditor from '@/components/record/CustomFieldsEditor.vue'
import FormSection from './FormSection.vue'
import TagPicker from './TagPicker.vue'
import StageSelect from './StageSelect.vue'
import { useAuth } from '@/stores/auth'
import { useSettings } from '@/stores/settings'
import { useToast } from '@/composables/useToast'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { customFields, toDecimal, toInt, useCustomFieldDefs, usePipelines } from './crm'

type Opportunity = Schemas['OpportunityView']

const props = defineProps<{
  open: boolean
  opportunity?: Opportunity | null
  preset?: { company_id?: string | null; company_name?: string | null; supplier_id?: string | null; supplier_name?: string | null }
}>()
const emit = defineEmits<{ close: []; saved: [opportunity: Opportunity] }>()
const auth = useAuth()
const toast = useToast()
const { label, settings } = useSettings()
const { errors, capture, clear } = useFieldErrors()
const { defs } = useCustomFieldDefs('opportunity')
const { stage } = usePipelines('opportunity')
const saving = ref(false)
const editing = computed(() => !!props.opportunity)

const blank = () => ({
  name: '', company_id: null as string | null, supplier_id: null as string | null, contact_id: null as string | null, pipeline_stage_id: null as string | null,
  amount: '', currency: '', expected_close_date: '', probability: '', outcome_reason: '', description: '', owner_id: null as string | null,
  custom_fields: {} as Record<string, unknown>, tag_ids: [] as string[],
})
const form = reactive(blank())
const closing = computed(() => {
  const s = stage(form.pipeline_stage_id)
  return s && s.category !== 'open' ? s.category : null
})

watch(
  () => props.open,
  (open) => {
    if (!open) return
    clear()
    Object.assign(form, blank())
    const o = props.opportunity
    if (o) {
      Object.assign(form, {
        name: o.name, company_id: o.company_id ?? null, supplier_id: o.supplier_id ?? null, contact_id: o.contact_id ?? null, pipeline_stage_id: o.pipeline_stage_id ?? null,
        amount: o.amount ?? '', currency: o.currency, expected_close_date: o.expected_close_date ? o.expected_close_date.slice(0, 10) : '',
        probability: o.probability === null || o.probability === undefined ? '' : String(o.probability), outcome_reason: o.outcome_reason,
        description: o.description, owner_id: o.owner_id ?? null, custom_fields: customFields(o.custom_fields),
      })
    } else {
      form.owner_id = auth.user?.id ?? null
      form.currency = settings.value?.imports.default_currency ?? ''
      form.company_id = props.preset?.company_id ?? null
      form.supplier_id = props.preset?.supplier_id ?? null
    }
  },
  { immediate: true },
)
// Settings may still be loading when the form opens.
watch(
  () => settings.value?.imports.default_currency,
  (c) => {
    if (props.open && !editing.value && !form.currency && c) form.currency = c
  },
)

async function submit() {
  const amount = toDecimal(form.amount)
  const probability = toInt(form.probability)
  const local: Record<string, string> = {}
  if (amount === undefined) local.amount = 'must be a number such as 1250.00'
  if (probability === undefined) local.probability = 'must be a whole number between 0 and 100'
  if (Object.keys(local).length) {
    errors.value = local
    return
  }
  saving.value = true
  const body: Schemas['OpportunityInput'] = {
    name: form.name, company_id: form.company_id, supplier_id: form.supplier_id, contact_id: form.contact_id, pipeline_stage_id: form.pipeline_stage_id,
    amount: amount ?? null, currency: form.currency.trim().toUpperCase(), expected_close_date: form.expected_close_date || null, probability: probability ?? null,
    outcome_reason: form.outcome_reason, description: form.description, owner_id: form.owner_id, custom_fields: form.custom_fields,
  }
  if (!editing.value && form.tag_ids.length) body.tag_ids = form.tag_ids
  try {
    const saved = editing.value ? await api.put<Opportunity>(`/opportunities/${props.opportunity!.id}`, body) : await api.post<Opportunity>('/opportunities', body)
    toast.success(`${label('opportunity')} ${editing.value ? 'updated' : 'created'}.`)
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
  <UiModal :open="open" :title="editing ? `Edit ${label('opportunity').toLowerCase()}` : `New ${label('opportunity').toLowerCase()}`" size="lg" @close="emit('close')">
    <form id="opportunity-form" class="space-y-6" novalidate @submit.prevent="submit">
      <FormSection title="Deal">
        <FormField v-slot="{ id }" label="Name" required :error="errors.name" class="sm:col-span-2">
          <input :id="id" v-model="form.name" class="input" maxlength="200" required />
        </FormField>
        <FormField v-slot="{ id }" label="Pipeline stage" :error="errors.pipeline_stage_id" hint="Won and lost stages close the deal.">
          <StageSelect v-model="form.pipeline_stage_id" entity-type="opportunity" :input-id="id" none-label="Default" />
        </FormField>
        <FormField v-slot="{ id }" label="Probability (%)" :error="errors.probability" hint="Empty uses the stage's probability.">
          <input :id="id" v-model="form.probability" class="input" inputmode="numeric" />
        </FormField>
        <FormField v-slot="{ id }" label="Amount" :error="errors.amount">
          <input :id="id" v-model="form.amount" class="input" inputmode="decimal" placeholder="0.00" />
        </FormField>
        <FormField v-slot="{ id }" label="Currency" :error="errors.currency" hint="ISO code, e.g. USD or EUR.">
          <input :id="id" v-model="form.currency" class="input uppercase" maxlength="3" autocomplete="off" />
        </FormField>
        <FormField v-slot="{ id }" label="Expected close date" :error="errors.expected_close_date">
          <input :id="id" v-model="form.expected_close_date" type="date" class="input" />
        </FormField>
        <FormField v-slot="{ id }" label="Owner" :error="errors.owner_id">
          <EntityPicker v-model="form.owner_id" type="user" :input-id="id" placeholder="No owner" :invalid="!!errors.owner_id" :initial-label="opportunity?.owner_name ?? (form.owner_id === auth.user?.id ? auth.user?.display_name : null)" />
        </FormField>
        <FormField v-if="closing || form.outcome_reason" v-slot="{ id }" :label="closing === 'lost' ? 'Reason lost' : closing === 'won' ? 'Reason won' : 'Outcome reason'" :error="errors.outcome_reason" class="sm:col-span-2">
          <input :id="id" v-model="form.outcome_reason" class="input" maxlength="1000" />
        </FormField>
      </FormSection>
      <FormSection title="Links">
        <FormField v-slot="{ id }" :label="label('company')" :error="errors.company_id">
          <EntityPicker v-model="form.company_id" type="company" :input-id="id" :invalid="!!errors.company_id" :initial-label="opportunity?.company_name ?? preset?.company_name ?? null" />
        </FormField>
        <FormField v-slot="{ id }" :label="label('supplier')" :error="errors.supplier_id">
          <EntityPicker v-model="form.supplier_id" type="supplier" :input-id="id" :invalid="!!errors.supplier_id" :initial-label="opportunity?.supplier_name ?? preset?.supplier_name ?? null" />
        </FormField>
        <FormField v-slot="{ id }" :label="label('contact')" :error="errors.contact_id">
          <EntityPicker v-model="form.contact_id" type="contact" :input-id="id" :invalid="!!errors.contact_id" :initial-label="opportunity?.contact_name ?? null" />
        </FormField>
        <FormField v-if="!editing" v-slot="{ id }" label="Tags" :error="errors.tag_ids">
          <TagPicker v-model="form.tag_ids" :input-id="id" />
        </FormField>
        <FormField v-slot="{ id }" label="Description" :error="errors.description" class="sm:col-span-2">
          <textarea :id="id" v-model="form.description" rows="4" class="input" maxlength="20000" />
        </FormField>
      </FormSection>
      <section v-if="defs.length" aria-label="Custom fields">
        <h3 class="mb-3 text-xs font-semibold tracking-wide text-slate-500 uppercase">Custom fields</h3>
        <CustomFieldsEditor v-model="form.custom_fields" entity-type="opportunity" :errors="errors" />
      </section>
      <p v-if="errors.custom_fields" class="text-xs text-red-600" role="alert">{{ errors.custom_fields }}</p>
    </form>
    <template #footer>
      <UiButton variant="ghost" @click="emit('close')">Cancel</UiButton>
      <UiButton type="submit" form="opportunity-form" variant="primary" :loading="saving">{{ editing ? 'Save changes' : `Create ${label('opportunity').toLowerCase()}` }}</UiButton>
    </template>
  </UiModal>
</template>
