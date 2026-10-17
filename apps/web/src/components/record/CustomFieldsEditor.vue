<script setup lang="ts">
// Renders the organization's custom field definitions for an entity type, as inputs or read-only.
import { computed } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { api, type Schemas } from '@/api/client'
import FormField from '@/components/ui/FormField.vue'
import { formatDate } from '@/lib/format'

const props = withDefaults(defineProps<{ entityType: string; readonly?: boolean; errors?: Record<string, string> }>(), { readonly: false, errors: () => ({}) })
const model = defineModel<Record<string, unknown>>({ default: () => ({}) })

const q = useQuery({
  queryKey: computed(() => ['custom-fields', props.entityType]),
  queryFn: ({ signal }) => api.get<Schemas['CustomFieldDefinitionItems']>('/custom-fields', { entity_type: props.entityType }, signal),
  staleTime: 60_000,
})
const defs = computed(() => [...(q.data.value?.items ?? [])].sort((a, b) => a.position - b.position))

function set(key: string, value: unknown) {
  const next = { ...model.value }
  if (value === '' || value === null || value === undefined) delete next[key]
  else next[key] = value
  model.value = next
}

function display(d: Schemas['CustomFieldDefinition']): string {
  const v = model.value[d.key]
  if (v === undefined || v === null || v === '') return '—'
  if (d.field_type === 'boolean') return v ? 'Yes' : 'No'
  if (d.field_type === 'date') return formatDate(String(v))
  return String(v)
}

function fieldError(key: string) {
  return props.errors[`custom_fields.${key}`] ?? props.errors[key]
}
</script>

<template>
  <div v-if="defs.length">
    <dl v-if="readonly" class="grid gap-x-6 gap-y-3 sm:grid-cols-2">
      <div v-for="d in defs" :key="d.id">
        <dt class="text-xs font-medium text-slate-500">{{ d.label }}</dt>
        <dd class="text-sm text-slate-800">
          <a v-if="d.field_type === 'url' && model[d.key]" :href="String(model[d.key])" target="_blank" rel="noopener noreferrer nofollow" class="text-brand-700 hover:underline">{{ model[d.key] }}</a>
          <template v-else>{{ display(d) }}</template>
        </dd>
      </div>
    </dl>
    <div v-else class="grid gap-4 sm:grid-cols-2">
      <FormField v-for="d in defs" :key="d.id" v-slot="{ id }" :label="d.label" :required="d.required" :error="fieldError(d.key)">
        <select v-if="d.field_type === 'select'" :id="id" class="input" :value="model[d.key] ?? ''" @change="set(d.key, ($event.target as HTMLSelectElement).value)">
          <option value="">—</option>
          <option v-for="o in d.options" :key="o" :value="o">{{ o }}</option>
        </select>
        <label v-else-if="d.field_type === 'boolean'" class="flex items-center gap-2 py-2 text-sm">
          <input :id="id" type="checkbox" class="rounded border-slate-300" :checked="model[d.key] === true" @change="set(d.key, ($event.target as HTMLInputElement).checked)" /> Yes
        </label>
        <input
          v-else-if="d.field_type === 'number'"
          :id="id"
          type="number"
          step="any"
          class="input"
          :value="model[d.key] ?? ''"
          @input="set(d.key, ($event.target as HTMLInputElement).value === '' ? null : Number(($event.target as HTMLInputElement).value))"
        />
        <input v-else :id="id" :type="d.field_type === 'date' ? 'date' : d.field_type === 'url' ? 'url' : 'text'" class="input" :value="model[d.key] ?? ''" @input="set(d.key, ($event.target as HTMLInputElement).value)" />
      </FormField>
    </div>
  </div>
</template>
