<script setup lang="ts">
// Create or edit a supplier (SupplierInput).
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
import { splitList, titleCase } from '@/lib/format'
import { SUPPLIER_TYPES, customFields, toInt, useCustomFieldDefs } from './crm'

type Supplier = Schemas['SupplierView']

const props = defineProps<{ open: boolean; supplier?: Supplier | null; preset?: { company_id?: string | null; company_name?: string | null } }>()
const emit = defineEmits<{ close: []; saved: [supplier: Supplier] }>()
const auth = useAuth()
const toast = useToast()
const { label } = useSettings()
const { errors, capture, clear } = useFieldErrors()
const { defs } = useCustomFieldDefs('supplier')
const saving = ref(false)
const editing = computed(() => !!props.supplier)

const blank = () => ({
  name: '', company_id: null as string | null, supplier_type: 'manufacturer', website: '', domain: '', country: '', city: '', address: '', email: '', phone: '',
  primary_contact_id: null as string | null, pipeline_stage_id: null as string | null, rating: '', payment_terms: '', lead_time_days: '', moq_notes: '',
  certifications: '', description: '', owner_id: null as string | null, custom_fields: {} as Record<string, unknown>, tag_ids: [] as string[],
})
const form = reactive(blank())

watch(
  () => props.open,
  (open) => {
    if (!open) return
    clear()
    Object.assign(form, blank())
    const s = props.supplier
    if (s) {
      Object.assign(form, {
        name: s.name, company_id: s.company_id ?? null, supplier_type: s.supplier_type, website: s.website ?? '', domain: s.domain ?? '', country: s.country,
        city: s.city, address: s.address, email: s.email, phone: s.phone, primary_contact_id: s.primary_contact_id ?? null, pipeline_stage_id: s.pipeline_stage_id ?? null,
        rating: s.rating ? String(s.rating) : '', payment_terms: s.payment_terms, lead_time_days: s.lead_time_days === null || s.lead_time_days === undefined ? '' : String(s.lead_time_days),
        moq_notes: s.moq_notes, certifications: (s.certifications ?? []).join(', '), description: s.description, owner_id: s.owner_id ?? null, custom_fields: customFields(s.custom_fields),
      })
    } else {
      form.owner_id = auth.user?.id ?? null
      form.company_id = props.preset?.company_id ?? null
    }
  },
  { immediate: true },
)

async function submit() {
  const leadTime = toInt(form.lead_time_days)
  if (leadTime === undefined) {
    errors.value = { lead_time_days: 'must be a whole number of days' }
    return
  }
  saving.value = true
  const body: Schemas['SupplierInput'] = {
    name: form.name, company_id: form.company_id, supplier_type: form.supplier_type, website: form.website.trim() || null, domain: form.domain.trim() || null,
    country: form.country, city: form.city, address: form.address, email: form.email, phone: form.phone, primary_contact_id: form.primary_contact_id,
    pipeline_stage_id: form.pipeline_stage_id, rating: form.rating ? Number(form.rating) : null, payment_terms: form.payment_terms, lead_time_days: leadTime,
    moq_notes: form.moq_notes, certifications: splitList(form.certifications), description: form.description, owner_id: form.owner_id, custom_fields: form.custom_fields,
  }
  if (!editing.value && form.tag_ids.length) body.tag_ids = form.tag_ids
  try {
    const saved = editing.value ? await api.put<Supplier>(`/suppliers/${props.supplier!.id}`, body) : await api.post<Supplier>('/suppliers', body)
    toast.success(`${label('supplier')} ${editing.value ? 'updated' : 'created'}.`)
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
  <UiModal :open="open" :title="editing ? `Edit ${label('supplier').toLowerCase()}` : `New ${label('supplier').toLowerCase()}`" size="lg" @close="emit('close')">
    <form id="supplier-form" class="space-y-6" novalidate @submit.prevent="submit">
      <FormSection title="Basics">
        <FormField v-slot="{ id }" label="Name" required :error="errors.name" class="sm:col-span-2">
          <input :id="id" v-model="form.name" class="input" maxlength="200" required />
        </FormField>
        <FormField v-slot="{ id }" label="Type" :error="errors.supplier_type">
          <select :id="id" v-model="form.supplier_type" class="input">
            <option v-for="t in SUPPLIER_TYPES" :key="t" :value="t">{{ titleCase(t) }}</option>
          </select>
        </FormField>
        <FormField v-slot="{ id }" label="Pipeline stage" :error="errors.pipeline_stage_id" hint="Empty uses the first stage of the default pipeline.">
          <StageSelect v-model="form.pipeline_stage_id" entity-type="supplier" :input-id="id" none-label="Default" />
        </FormField>
        <FormField v-slot="{ id }" label="Rating" :error="errors.rating">
          <select :id="id" v-model="form.rating" class="input">
            <option value="">Not rated</option>
            <option v-for="n in 5" :key="n" :value="String(n)">{{ n }} / 5</option>
          </select>
        </FormField>
        <FormField v-slot="{ id }" label="Owner" :error="errors.owner_id">
          <EntityPicker v-model="form.owner_id" type="user" :input-id="id" placeholder="No owner" :invalid="!!errors.owner_id" :initial-label="form.owner_id === auth.user?.id ? auth.user?.display_name : null" />
        </FormField>
        <FormField v-slot="{ id }" label="Website" :error="errors.website">
          <input :id="id" v-model="form.website" class="input" inputmode="url" placeholder="https://example.com" />
        </FormField>
        <FormField v-slot="{ id }" label="Domain" :error="errors.domain" hint="Filled from the website when empty.">
          <input :id="id" v-model="form.domain" class="input" placeholder="example.com" />
        </FormField>
        <FormField v-slot="{ id }" :label="label('company')" :error="errors.company_id">
          <EntityPicker v-model="form.company_id" type="company" :input-id="id" :invalid="!!errors.company_id" :initial-label="supplier?.company_name ?? preset?.company_name ?? null" />
        </FormField>
        <FormField v-slot="{ id }" :label="`Primary ${label('contact').toLowerCase()}`" :error="errors.primary_contact_id">
          <EntityPicker v-model="form.primary_contact_id" type="contact" :input-id="id" :invalid="!!errors.primary_contact_id" :initial-label="supplier?.primary_contact_name ?? null" />
        </FormField>
        <FormField v-if="!editing" v-slot="{ id }" label="Tags" :error="errors.tag_ids">
          <TagPicker v-model="form.tag_ids" :input-id="id" />
        </FormField>
      </FormSection>
      <FormSection title="Sourcing terms">
        <FormField v-slot="{ id }" label="Payment terms" :error="errors.payment_terms">
          <input :id="id" v-model="form.payment_terms" class="input" maxlength="300" placeholder="30% deposit, 70% before shipping" />
        </FormField>
        <FormField v-slot="{ id }" label="Lead time (days)" :error="errors.lead_time_days">
          <input :id="id" v-model="form.lead_time_days" class="input" inputmode="numeric" />
        </FormField>
        <FormField v-slot="{ id }" label="Certifications" :error="errors.certifications" hint="Separate with commas." class="sm:col-span-2">
          <input :id="id" v-model="form.certifications" class="input" placeholder="ISO 9001, CE" />
        </FormField>
        <FormField v-slot="{ id }" label="MOQ notes" :error="errors.moq_notes" class="sm:col-span-2">
          <textarea :id="id" v-model="form.moq_notes" rows="2" class="input" maxlength="2000" />
        </FormField>
      </FormSection>
      <FormSection title="Contact details">
        <FormField v-slot="{ id }" label="E-mail" :error="errors.email">
          <input :id="id" v-model="form.email" type="email" class="input" />
        </FormField>
        <FormField v-slot="{ id }" label="Phone" :error="errors.phone">
          <input :id="id" v-model="form.phone" type="tel" class="input" maxlength="60" />
        </FormField>
        <FormField v-slot="{ id }" label="Country" :error="errors.country">
          <input :id="id" v-model="form.country" class="input" maxlength="80" />
        </FormField>
        <FormField v-slot="{ id }" label="City" :error="errors.city">
          <input :id="id" v-model="form.city" class="input" maxlength="120" />
        </FormField>
        <FormField v-slot="{ id }" label="Address" :error="errors.address" class="sm:col-span-2">
          <input :id="id" v-model="form.address" class="input" maxlength="500" />
        </FormField>
        <FormField v-slot="{ id }" label="Description" :error="errors.description" class="sm:col-span-2">
          <textarea :id="id" v-model="form.description" rows="4" class="input" maxlength="20000" />
        </FormField>
      </FormSection>
      <section v-if="defs.length" aria-label="Custom fields">
        <h3 class="mb-3 text-xs font-semibold tracking-wide text-slate-500 uppercase">Custom fields</h3>
        <CustomFieldsEditor v-model="form.custom_fields" entity-type="supplier" :errors="errors" />
      </section>
      <p v-if="errors.custom_fields" class="text-xs text-red-600" role="alert">{{ errors.custom_fields }}</p>
    </form>
    <template #footer>
      <UiButton variant="ghost" @click="emit('close')">Cancel</UiButton>
      <UiButton type="submit" form="supplier-form" variant="primary" :loading="saving">{{ editing ? 'Save changes' : `Create ${label('supplier').toLowerCase()}` }}</UiButton>
    </template>
  </UiModal>
</template>
