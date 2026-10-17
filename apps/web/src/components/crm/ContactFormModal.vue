<script setup lang="ts">
// Create or edit a contact (ContactInput), optionally preset to a company or supplier.
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
import { RECORD_STATUSES, customFields, useCustomFieldDefs } from './crm'

type Contact = Schemas['ContactView']

const props = defineProps<{
  open: boolean
  contact?: Contact | null
  /** Preset links for a new contact created from a company or supplier page. */
  preset?: { company_id?: string | null; company_name?: string | null; supplier_id?: string | null; supplier_name?: string | null }
}>()
const emit = defineEmits<{ close: []; saved: [contact: Contact] }>()
const auth = useAuth()
const toast = useToast()
const { label } = useSettings()
const { errors, capture, clear } = useFieldErrors()
const { defs } = useCustomFieldDefs('contact')
const saving = ref(false)
const editing = computed(() => !!props.contact)

const blank = () => ({
  first_name: '', last_name: '', email: '', phone: '', job_title: '', company_id: null as string | null, supplier_id: null as string | null,
  is_decision_maker: false, linkedin_url: '', country: '', status: 'active', notes_summary: '', owner_id: null as string | null,
  custom_fields: {} as Record<string, unknown>, tag_ids: [] as string[],
})
const form = reactive(blank())

watch(
  () => props.open,
  (open) => {
    if (!open) return
    clear()
    Object.assign(form, blank())
    const c = props.contact
    if (c) {
      Object.assign(form, {
        first_name: c.first_name, last_name: c.last_name, email: c.email ?? '', phone: c.phone, job_title: c.job_title, company_id: c.company_id ?? null,
        supplier_id: c.supplier_id ?? null, is_decision_maker: c.is_decision_maker, linkedin_url: c.linkedin_url, country: c.country, status: c.status,
        notes_summary: c.notes_summary, owner_id: c.owner_id ?? null, custom_fields: customFields(c.custom_fields),
      })
    } else {
      form.owner_id = auth.user?.id ?? null
      form.company_id = props.preset?.company_id ?? null
      form.supplier_id = props.preset?.supplier_id ?? null
    }
  },
  { immediate: true },
)

async function submit() {
  saving.value = true
  const body: Schemas['ContactInput'] = {
    first_name: form.first_name, last_name: form.last_name, email: form.email.trim() || null, phone: form.phone, job_title: form.job_title,
    company_id: form.company_id, supplier_id: form.supplier_id, is_decision_maker: form.is_decision_maker, linkedin_url: form.linkedin_url,
    country: form.country, status: form.status, notes_summary: form.notes_summary, owner_id: form.owner_id, custom_fields: form.custom_fields,
  }
  if (!editing.value && form.tag_ids.length) body.tag_ids = form.tag_ids
  try {
    const saved = editing.value ? await api.put<Contact>(`/contacts/${props.contact!.id}`, body) : await api.post<Contact>('/contacts', body)
    toast.success(`${label('contact')} ${editing.value ? 'updated' : 'created'}.`)
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
  <UiModal :open="open" :title="editing ? `Edit ${label('contact').toLowerCase()}` : `New ${label('contact').toLowerCase()}`" size="lg" @close="emit('close')">
    <form id="contact-form" class="space-y-6" novalidate @submit.prevent="submit">
      <FormSection title="Person">
        <FormField v-slot="{ id }" label="First name" :error="errors.first_name" hint="A first or last name is required.">
          <input :id="id" v-model="form.first_name" class="input" maxlength="100" autocomplete="given-name" />
        </FormField>
        <FormField v-slot="{ id }" label="Last name" :error="errors.last_name">
          <input :id="id" v-model="form.last_name" class="input" maxlength="100" autocomplete="family-name" />
        </FormField>
        <FormField v-slot="{ id }" label="E-mail" :error="errors.email">
          <input :id="id" v-model="form.email" type="email" class="input" autocomplete="email" />
        </FormField>
        <FormField v-slot="{ id }" label="Phone" :error="errors.phone">
          <input :id="id" v-model="form.phone" type="tel" class="input" maxlength="60" />
        </FormField>
        <FormField v-slot="{ id }" label="Job title" :error="errors.job_title">
          <input :id="id" v-model="form.job_title" class="input" maxlength="150" />
        </FormField>
        <FormField v-slot="{ id }" label="Status" :error="errors.status">
          <select :id="id" v-model="form.status" class="input">
            <option v-for="s in RECORD_STATUSES" :key="s" :value="s">{{ titleCase(s) }}</option>
          </select>
        </FormField>
        <FormField v-slot="{ id }" label="LinkedIn URL" :error="errors.linkedin_url">
          <input :id="id" v-model="form.linkedin_url" type="url" class="input" placeholder="https://www.linkedin.com/in/…" />
        </FormField>
        <FormField v-slot="{ id }" label="Country" :error="errors.country">
          <input :id="id" v-model="form.country" class="input" maxlength="80" />
        </FormField>
        <div class="sm:col-span-2">
          <label class="flex items-center gap-2 text-sm text-slate-700">
            <input v-model="form.is_decision_maker" type="checkbox" class="rounded border-slate-300" /> Decision maker
          </label>
        </div>
      </FormSection>
      <FormSection title="Links">
        <FormField v-slot="{ id }" :label="label('company')" :error="errors.company_id">
          <EntityPicker v-model="form.company_id" type="company" :input-id="id" :invalid="!!errors.company_id" :initial-label="contact?.company_name ?? preset?.company_name ?? null" />
        </FormField>
        <FormField v-slot="{ id }" :label="label('supplier')" :error="errors.supplier_id">
          <EntityPicker v-model="form.supplier_id" type="supplier" :input-id="id" :invalid="!!errors.supplier_id" :initial-label="contact?.supplier_name ?? preset?.supplier_name ?? null" />
        </FormField>
        <FormField v-slot="{ id }" label="Owner" :error="errors.owner_id">
          <EntityPicker v-model="form.owner_id" type="user" :input-id="id" placeholder="No owner" :invalid="!!errors.owner_id" :initial-label="form.owner_id === auth.user?.id ? auth.user?.display_name : null" />
        </FormField>
        <FormField v-if="!editing" v-slot="{ id }" label="Tags" :error="errors.tag_ids">
          <TagPicker v-model="form.tag_ids" :input-id="id" />
        </FormField>
        <FormField v-slot="{ id }" label="Summary" :error="errors.notes_summary" class="sm:col-span-2">
          <textarea :id="id" v-model="form.notes_summary" rows="3" class="input" maxlength="5000" />
        </FormField>
      </FormSection>
      <section v-if="defs.length" aria-label="Custom fields">
        <h3 class="mb-3 text-xs font-semibold tracking-wide text-slate-500 uppercase">Custom fields</h3>
        <CustomFieldsEditor v-model="form.custom_fields" entity-type="contact" :errors="errors" />
      </section>
      <p v-if="errors.custom_fields" class="text-xs text-red-600" role="alert">{{ errors.custom_fields }}</p>
    </form>
    <template #footer>
      <UiButton variant="ghost" @click="emit('close')">Cancel</UiButton>
      <UiButton type="submit" form="contact-form" variant="primary" :loading="saving">{{ editing ? 'Save changes' : `Create ${label('contact').toLowerCase()}` }}</UiButton>
    </template>
  </UiModal>
</template>
