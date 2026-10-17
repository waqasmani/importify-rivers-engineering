<script setup lang="ts">
// Project details shared by the new-analysis wizard and the project edit dialog.
import { computed } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { api, errorMessage, type Schemas } from '@/api/client'
import FormField from '@/components/ui/FormField.vue'
import EntityPicker from '@/components/record/EntityPicker.vue'
import { ANALYSIS_TYPES, PROJECT_STATUSES } from './options'

export interface ProjectFieldsModel {
  name: string
  workspace_id: string
  description: string
  objective: string
  status: string
  analysis_types: string[]
  owner_id: string | null
}

const model = defineModel<ProjectFieldsModel>({ required: true })
withDefaults(defineProps<{ errors?: Record<string, string> }>(), { errors: () => ({}) })

const workspaces = useQuery({
  queryKey: ['workspaces'],
  queryFn: ({ signal }) => api.get<Schemas['ListWorkspacesRowItems']>('/workspaces', undefined, signal),
  staleTime: 60_000,
})
const wsItems = computed(() => workspaces.data.value?.items ?? [])
</script>

<template>
  <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
    <FormField v-slot="{ id, describedby, invalid }" label="Project name" required :error="errors.name" class="sm:col-span-2">
      <input :id="id" v-model="model.name" class="input" :class="{ 'input-invalid': invalid }" maxlength="200" required :aria-describedby="describedby" :aria-invalid="invalid" />
    </FormField>
    <FormField v-slot="{ id, describedby, invalid }" label="Workspace" required :error="errors.workspace_id ?? (workspaces.error.value ? errorMessage(workspaces.error.value) : undefined)">
      <select :id="id" v-model="model.workspace_id" class="input" :class="{ 'input-invalid': invalid }" required :aria-describedby="describedby" :aria-invalid="invalid">
        <option value="" disabled>{{ workspaces.isLoading.value ? 'Loading…' : wsItems.length ? 'Choose a workspace' : 'No workspaces available' }}</option>
        <option v-for="w in wsItems" :key="w.id" :value="w.id">{{ w.name }}</option>
      </select>
    </FormField>
    <FormField v-slot="{ id, describedby }" label="Status" :error="errors.status">
      <select :id="id" v-model="model.status" class="input" :aria-describedby="describedby">
        <option v-for="s in PROJECT_STATUSES" :key="s" :value="s">{{ s.charAt(0).toUpperCase() + s.slice(1) }}</option>
      </select>
    </FormField>
    <FormField v-slot="{ id, describedby }" label="Objective" hint="What you want to learn, for example “Document the public product API before an integration”." :error="errors.objective" class="sm:col-span-2">
      <textarea :id="id" v-model="model.objective" rows="2" class="input" maxlength="5000" :aria-describedby="describedby" />
    </FormField>
    <FormField v-slot="{ id, describedby }" label="Description" :error="errors.description" class="sm:col-span-2">
      <textarea :id="id" v-model="model.description" rows="3" class="input" maxlength="10000" :aria-describedby="describedby" />
    </FormField>
    <fieldset class="sm:col-span-2">
      <legend class="label">Analysis types</legend>
      <div class="flex flex-wrap gap-2">
        <label v-for="t in ANALYSIS_TYPES" :key="t.value" class="flex items-center gap-1.5 rounded-full border border-slate-200 bg-white px-3 py-1 text-sm has-[:checked]:border-brand-500 has-[:checked]:bg-brand-50">
          <input v-model="model.analysis_types" type="checkbox" :value="t.value" class="rounded border-slate-300" /> {{ t.label }}
        </label>
      </div>
      <p v-if="errors.analysis_types" class="mt-1 text-xs text-red-600" role="alert">{{ errors.analysis_types }}</p>
    </fieldset>
    <FormField v-slot="{ id }" label="Owner" hint="Optional. The person responsible for this analysis." :error="errors.owner_id">
      <EntityPicker v-model="model.owner_id" type="user" :input-id="id" placeholder="Search people" :invalid="!!errors.owner_id" />
    </FormField>
  </div>
</template>
