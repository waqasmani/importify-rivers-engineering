<script setup lang="ts">
// Create or edit a lead (LeadInput). The estimated value is a decimal string with an ISO currency.
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
import { titleCase } from '@/lib/format'
import { LEAD_SOURCES, customFields, toDecimal, useCustomFieldDefs } from './crm'

type Lead = Schemas['LeadView']

const props = defineProps<{ open: boolean; lead?: Lead | null; preset?: { company_id?: string | null; company_name?: string | null; contact_id?: string | null } }>()
const emit = defineEmits<{ close: []; saved: [lead: Lead] }>()
const auth = useAuth()
const toast = useToast()
const { label, settings } = useSettings()
const { errors, capture, clear } = useFieldErrors()
const { defs } = useCustomFieldDefs('lead')
const saving = ref(false)
const editing = computed(() => !!props.lead)

const blank = () => ({
  title: '', company_id: null as string | null, contact_id: null as string | null, source: 'website', pipeline_stage_id: null as string | null,
  estimated_value: '', currency: '', description: '', owner_id: null as string | null, custom_fields: {} as Record<string, unknown>, tag_ids: [] as string[],
})
const form = reactive(blank())

watch(
  () => props.open,
  (open) => {
    if (!open) return
    clear()
    Object.assign(form, blank())
    const l = props.lead
    if (l) {
      Object.assign(form, {
        title: l.title, company_id: l.company_id ?? null, contact_id: l.contact_id ?? null, source: l.source, pipeline_stage_id: l.pipeline_stage_id ?? null,
        estimated_value: l.estimated_value ?? '', currency: l.currency, description: l.description, owner_id: l.owner_id ?? null, custom_fields: customFields(l.custom_fields),
      })
    } else {
      form.owner_id = auth.user?.id ?? null
      form.currency = settings.value?.imports.default_currency ?? ''
      form.company_id = props.preset?.company_id ?? null
      form.contact_id = props.preset?.contact_id ?? null
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
  const value = toDecimal(form.estimated_value)
  if (value === undefined) {
    errors.value = { estimated_value: 'must be a number such as 1250.00' }
    return
  }
  saving.value = true
  const body: Schemas['LeadInput'] = {
    title: form.title, company_id: form.company_id, contact_id: form.contact_id, source: form.source, pipeline_stage_id: form.pipeline_stage_id,
    estimated_value: value, currency: form.currency.trim().toUpperCase(), description: form.description, owner_id: form.owner_id, custom_fields: form.custom_fields,
  }
  if (!editing.value && form.tag_ids.length) body.tag_ids = form.tag_ids
  try {
    const saved = editing.value ? await api.put<Lead>(`/leads/${props.lead!.id}`, body) : await api.post<Lead>('/leads', body)
    toast.success(`${label('lead')} ${editing.value ? 'updated' : 'created'}.`)
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
  <UiModal :open="open" :title="editing ? `Edit ${label('lead').toLowerCase()}` : `New ${label('lead').toLowerCase()}`" size="lg" @close="emit('close')">
    <form id="lead-form" class="space-y-6" novalidate @submit.prevent="submit">
      <FormSection title="Basics">
        <FormField v-slot="{ id }" label="Title" required :error="errors.title" class="sm:col-span-2">
          <input :id="id" v-model="form.title" class="input" maxlength="200" required />
        </FormField>
        <FormField v-slot="{ id }" label="Source" :error="errors.source">
          <select :id="id" v-model="form.source" class="input">
            <option v-for="s in LEAD_SOURCES" :key="s" :value="s">{{ titleCase(s) }}</option>
          </select>
        </FormField>
        <FormField v-slot="{ id }" label="Pipeline stage" :error="errors.pipeline_stage_id" hint="Empty uses the first stage of the default pipeline.">
          <StageSelect v-model="form.pipeline_stage_id" entity-type="lead" :input-id="id" none-label="Default" />
        </FormField>
        <FormField v-slot="{ id }" label="Estimated value" :error="errors.estimated_value">
          <input :id="id" v-model="form.estimated_value" class="input" inputmode="decimal" placeholder="0.00" />
        </FormField>
        <FormField v-slot="{ id }" label="Currency" :error="errors.currency" hint="ISO code, e.g. USD or EUR.">
          <input :id="id" v-model="form.currency" class="input uppercase" maxlength="3" autocomplete="off" />
        </FormField>
        <FormField v-slot="{ id }" :label="label('company')" :error="errors.company_id">
          <EntityPicker v-model="form.company_id" type="company" :input-id="id" :invalid="!!errors.company_id" :initial-label="lead?.company_name ?? preset?.company_name ?? null" />
        </FormField>
        <FormField v-slot="{ id }" :label="label('contact')" :error="errors.contact_id">
          <EntityPicker v-model="form.contact_id" type="contact" :input-id="id" :invalid="!!errors.contact_id" :initial-label="lead?.contact_name ?? null" />
        </FormField>
        <FormField v-slot="{ id }" label="Owner" :error="errors.owner_id">
          <EntityPicker v-model="form.owner_id" type="user" :input-id="id" placeholder="No owner" :invalid="!!errors.owner_id" :initial-label="lead?.owner_name ?? (form.owner_id === auth.user?.id ? auth.user?.display_name : null)" />
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
        <CustomFieldsEditor v-model="form.custom_fields" entity-type="lead" :errors="errors" />
      </section>
      <p v-if="errors.custom_fields" class="text-xs text-red-600" role="alert">{{ errors.custom_fields }}</p>
    </form>
    <template #footer>
      <UiButton variant="ghost" @click="emit('close')">Cancel</UiButton>
      <UiButton type="submit" form="lead-form" variant="primary" :loading="saving">{{ editing ? 'Save changes' : `Create ${label('lead').toLowerCase()}` }}</UiButton>
    </template>
  </UiModal>
</template>
