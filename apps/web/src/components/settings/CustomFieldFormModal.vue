<script setup lang="ts">
// Create or edit a custom field definition. The key is fixed once created because stored values
// on records are keyed by it.
import { reactive, ref, watch } from 'vue'
import { ArrowDown, ArrowUp, Plus, X } from 'lucide-vue-next'
import { api, type Schemas } from '@/api/client'
import UiModal from '@/components/ui/UiModal.vue'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { useToast } from '@/composables/useToast'
import { CUSTOM_FIELD_TYPES } from '@/lib/options'
import { keyFrom } from './options'

type Field = Schemas['CustomFieldDefinition']

const props = defineProps<{ open: boolean; field: Field | null; entityType: string; entityLabel: string; nextPosition: number }>()
const emit = defineEmits<{ close: []; saved: [field: Field] }>()
const toast = useToast()
const { errors, capture, clear } = useFieldErrors()
const saving = ref(false)

const typeLabels: Record<string, string> = { text: 'Text', number: 'Number', date: 'Date', boolean: 'Yes / no', select: 'Choice list', url: 'Web address' }
const form = reactive({ label: '', key: '', field_type: 'text', options: [] as string[], required: false, position: 0 })
const keyTouched = ref(false)
const newOption = ref('')

watch(
  () => props.open,
  (open) => {
    if (!open) return
    clear()
    const f = props.field
    form.label = f?.label ?? ''
    form.key = f?.key ?? ''
    form.field_type = f?.field_type ?? 'text'
    form.options = [...(f?.options ?? [])]
    form.required = f?.required ?? false
    form.position = f?.position ?? props.nextPosition
    keyTouched.value = !!f
    newOption.value = ''
  },
  { immediate: true },
)
watch(
  () => form.label,
  (l) => {
    if (!keyTouched.value) form.key = keyFrom(l)
  },
)

function addOption() {
  const v = newOption.value.trim()
  if (!v) return
  if (!form.options.some((o) => o.toLowerCase() === v.toLowerCase())) form.options.push(v)
  newOption.value = ''
}
function moveOption(i: number, d: -1 | 1) {
  const j = i + d
  if (j < 0 || j >= form.options.length) return
  const o = [...form.options]
  ;[o[i], o[j]] = [o[j]!, o[i]!]
  form.options = o
}

async function submit() {
  if (newOption.value.trim()) addOption()
  saving.value = true
  clear()
  const body: Schemas['CustomFieldInput'] = {
    entity_type: props.field?.entity_type ?? props.entityType,
    key: props.field?.key ?? form.key,
    label: form.label,
    field_type: form.field_type,
    options: form.field_type === 'select' ? form.options : [],
    required: form.required,
    position: Number(form.position) || 0,
  }
  try {
    const saved = props.field ? await api.put<Field>(`/custom-fields/${props.field.id}`, body) : await api.post<Field>('/custom-fields', body)
    toast.success(props.field ? 'Field saved.' : 'Field created.')
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
  <UiModal :open="open" :title="field ? `Edit ${field.label}` : `New ${entityLabel.toLowerCase()} field`" @close="emit('close')">
    <form id="custom-field-form" class="grid grid-cols-1 gap-4 sm:grid-cols-2" @submit.prevent="submit">
      <FormField v-slot="{ id }" label="Label" required :error="errors.label" class="sm:col-span-2">
        <input :id="id" v-model="form.label" class="input" maxlength="80" required />
      </FormField>
      <FormField v-slot="{ id, describedby }" label="Key" :required="!field" :error="errors.key" :hint="field ? 'The key cannot be changed after creation.' : 'Used in the API, CSV import and exports. Lowercase letters, digits and underscores.'">
        <input
          :id="id"
          v-model="form.key"
          class="input font-mono"
          maxlength="41"
          pattern="[a-z][a-z0-9_]*"
          :disabled="!!field"
          :required="!field"
          :aria-describedby="describedby"
          @input="keyTouched = true"
        />
      </FormField>
      <FormField v-slot="{ id, describedby }" label="Type" :error="errors.field_type" :hint="field ? 'Changing the type does not convert values already stored on records.' : undefined">
        <select :id="id" v-model="form.field_type" class="input" :aria-describedby="describedby">
          <option v-for="t in CUSTOM_FIELD_TYPES" :key="t" :value="t">{{ typeLabels[t] ?? t }}</option>
        </select>
      </FormField>

      <fieldset v-if="form.field_type === 'select'" class="sm:col-span-2">
        <legend class="label">Choices</legend>
        <ol v-if="form.options.length" class="mb-2 divide-y divide-slate-100 rounded-md border border-slate-200">
          <li v-for="(o, i) in form.options" :key="o" class="flex items-center gap-2 px-2 py-1 text-sm" data-testid="option-row">
            <span class="flex-1 truncate">{{ o }}</span>
            <button type="button" class="rounded p-1 text-slate-500 hover:bg-slate-100 disabled:opacity-30" :disabled="i === 0" :aria-label="`Move ${o} up`" @click="moveOption(i, -1)"><ArrowUp class="h-3.5 w-3.5" /></button>
            <button type="button" class="rounded p-1 text-slate-500 hover:bg-slate-100 disabled:opacity-30" :disabled="i === form.options.length - 1" :aria-label="`Move ${o} down`" @click="moveOption(i, 1)"><ArrowDown class="h-3.5 w-3.5" /></button>
            <button type="button" class="rounded p-1 text-slate-400 hover:bg-red-50 hover:text-red-600" :aria-label="`Remove ${o}`" @click="form.options.splice(i, 1)"><X class="h-3.5 w-3.5" /></button>
          </li>
        </ol>
        <div class="flex gap-2">
          <label for="cf-new-option" class="sr-only">New choice</label>
          <input id="cf-new-option" v-model="newOption" class="input" maxlength="200" placeholder="Add a choice and press Enter" @keydown.enter.prevent="addOption" />
          <UiButton :disabled="!newOption.trim() || form.options.length >= 100" @click="addOption"><Plus class="h-4 w-4" aria-hidden="true" /> Add</UiButton>
        </div>
        <p v-if="errors.options" class="mt-1 text-xs text-red-600" role="alert">{{ errors.options }}</p>
        <p v-else class="mt-1 text-xs text-slate-500">Between 1 and 100 choices.</p>
      </fieldset>

      <FormField v-slot="{ id, describedby }" label="Position" :error="errors.position" hint="Fields are shown in ascending order.">
        <input :id="id" v-model.number="form.position" type="number" step="1" class="input" :aria-describedby="describedby" />
      </FormField>
      <label class="flex items-center gap-2 self-end pb-2 text-sm text-slate-700">
        <input v-model="form.required" type="checkbox" class="rounded border-slate-300" /> Required: records cannot be saved without a value
      </label>
    </form>
    <template #footer>
      <UiButton variant="ghost" @click="emit('close')">Cancel</UiButton>
      <UiButton type="submit" form="custom-field-form" variant="primary" :loading="saving">{{ field ? 'Save field' : 'Create field' }}</UiButton>
    </template>
  </UiModal>
</template>
