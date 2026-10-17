<script setup lang="ts">
// Create or edit a company (CompanyInput). Tags can be chosen on create; afterwards TagEditor manages them.
import { computed, reactive, ref, watch } from 'vue'
import { api, type Schemas } from '@/api/client'
import UiModal from '@/components/ui/UiModal.vue'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import EntityPicker from '@/components/record/EntityPicker.vue'
import CustomFieldsEditor from '@/components/record/CustomFieldsEditor.vue'
import FormSection from './FormSection.vue'
import TagPicker from './TagPicker.vue'
import { useAuth } from '@/stores/auth'
import { useSettings } from '@/stores/settings'
import { useToast } from '@/composables/useToast'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { titleCase } from '@/lib/format'
import { COMPANY_TYPES, RECORD_STATUSES, customFields, toInt, useCustomFieldDefs } from './crm'

type Company = Schemas['CompanyView']

const props = defineProps<{ open: boolean; company?: Company | null }>()
const emit = defineEmits<{ close: []; saved: [company: Company] }>()
const auth = useAuth()
const toast = useToast()
const { label } = useSettings()
const { errors, capture, clear } = useFieldErrors()
const { defs } = useCustomFieldDefs('company')
const saving = ref(false)
const editing = computed(() => !!props.company)

const blank = () => ({
  name: '', domain: '', website: '', industry: '', company_type: 'prospect', status: 'active', country: '', city: '', address: '', phone: '', email: '',
  description: '', employee_count: '', owner_id: null as string | null, custom_fields: {} as Record<string, unknown>, tag_ids: [] as string[],
})
const form = reactive(blank())

watch(
  () => props.open,
  (open) => {
    if (!open) return
    clear()
    const c = props.company
    Object.assign(form, blank())
    if (c) {
      Object.assign(form, {
        name: c.name, domain: c.domain ?? '', website: c.website ?? '', industry: c.industry, company_type: c.company_type, status: c.status,
        country: c.country, city: c.city, address: c.address, phone: c.phone, email: c.email, description: c.description,
        employee_count: c.employee_count === null || c.employee_count === undefined ? '' : String(c.employee_count),
        owner_id: c.owner_id ?? null, custom_fields: customFields(c.custom_fields),
      })
    } else {
      form.owner_id = auth.user?.id ?? null
    }
  },
  { immediate: true },
)

async function submit() {
  const employees = toInt(form.employee_count)
  if (employees === undefined) {
    errors.value = { employee_count: 'must be a whole number' }
    return
  }
  saving.value = true
  const body: Schemas['CompanyInput'] = {
    name: form.name, domain: form.domain.trim() || null, website: form.website.trim() || null, industry: form.industry, company_type: form.company_type,
    status: form.status, country: form.country, city: form.city, address: form.address, phone: form.phone, email: form.email,
    description: form.description, employee_count: employees, owner_id: form.owner_id, custom_fields: form.custom_fields,
  }
  if (!editing.value && form.tag_ids.length) body.tag_ids = form.tag_ids
  try {
    const saved = editing.value ? await api.put<Company>(`/companies/${props.company!.id}`, body) : await api.post<Company>('/companies', body)
    toast.success(`${label('company')} ${editing.value ? 'updated' : 'created'}.`)
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
  <UiModal :open="open" :title="editing ? `Edit ${label('company').toLowerCase()}` : `New ${label('company').toLowerCase()}`" size="lg" @close="emit('close')">
    <form id="company-form" class="space-y-6" novalidate @submit.prevent="submit">
      <FormSection title="Basics">
        <FormField v-slot="{ id }" label="Name" required :error="errors.name" class="sm:col-span-2">
          <input :id="id" v-model="form.name" class="input" maxlength="200" required autocomplete="organization" />
        </FormField>
        <FormField v-slot="{ id }" label="Type" :error="errors.company_type">
          <select :id="id" v-model="form.company_type" class="input">
            <option v-for="t in COMPANY_TYPES" :key="t" :value="t">{{ titleCase(t) }}</option>
          </select>
        </FormField>
        <FormField v-slot="{ id }" label="Status" :error="errors.status">
          <select :id="id" v-model="form.status" class="input">
            <option v-for="s in RECORD_STATUSES" :key="s" :value="s">{{ titleCase(s) }}</option>
          </select>
        </FormField>
        <FormField v-slot="{ id }" label="Website" :error="errors.website" hint="The domain is filled from the website when left empty.">
          <input :id="id" v-model="form.website" class="input" inputmode="url" placeholder="https://example.com" />
        </FormField>
        <FormField v-slot="{ id }" label="Domain" :error="errors.domain">
          <input :id="id" v-model="form.domain" class="input" placeholder="example.com" />
        </FormField>
        <FormField v-slot="{ id }" label="Industry" :error="errors.industry">
          <input :id="id" v-model="form.industry" class="input" maxlength="120" />
        </FormField>
        <FormField v-slot="{ id }" label="Employees" :error="errors.employee_count">
          <input :id="id" v-model="form.employee_count" class="input" inputmode="numeric" />
        </FormField>
        <FormField v-slot="{ id }" label="Owner" :error="errors.owner_id">
          <EntityPicker v-model="form.owner_id" type="user" :input-id="id" placeholder="No owner" :invalid="!!errors.owner_id" :initial-label="company?.owner_name ?? (form.owner_id === auth.user?.id ? auth.user?.display_name : null)" />
        </FormField>
        <FormField v-if="!editing" v-slot="{ id }" label="Tags" :error="errors.tag_ids">
          <TagPicker v-model="form.tag_ids" :input-id="id" />
        </FormField>
      </FormSection>
      <FormSection title="Contact details">
        <FormField v-slot="{ id }" label="E-mail" :error="errors.email">
          <input :id="id" v-model="form.email" type="email" class="input" autocomplete="email" />
        </FormField>
        <FormField v-slot="{ id }" label="Phone" :error="errors.phone">
          <input :id="id" v-model="form.phone" type="tel" class="input" maxlength="60" />
        </FormField>
        <FormField v-slot="{ id }" label="Country" :error="errors.country">
          <input :id="id" v-model="form.country" class="input" maxlength="80" autocomplete="country-name" />
        </FormField>
        <FormField v-slot="{ id }" label="City" :error="errors.city">
          <input :id="id" v-model="form.city" class="input" maxlength="120" />
        </FormField>
        <FormField v-slot="{ id }" label="Address" :error="errors.address" class="sm:col-span-2">
          <input :id="id" v-model="form.address" class="input" maxlength="500" autocomplete="street-address" />
        </FormField>
        <FormField v-slot="{ id }" label="Description" :error="errors.description" class="sm:col-span-2">
          <textarea :id="id" v-model="form.description" rows="4" class="input" maxlength="20000" />
        </FormField>
      </FormSection>
      <section v-if="defs.length" aria-label="Custom fields">
        <h3 class="mb-3 text-xs font-semibold tracking-wide text-slate-500 uppercase">Custom fields</h3>
        <CustomFieldsEditor v-model="form.custom_fields" entity-type="company" :errors="errors" />
      </section>
      <p v-if="errors.custom_fields" class="text-xs text-red-600" role="alert">{{ errors.custom_fields }}</p>
    </form>
    <template #footer>
      <UiButton variant="ghost" @click="emit('close')">Cancel</UiButton>
      <UiButton type="submit" form="company-form" variant="primary" :loading="saving">{{ editing ? 'Save changes' : `Create ${label('company').toLowerCase()}` }}</UiButton>
    </template>
  </UiModal>
</template>
