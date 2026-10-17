<script setup lang="ts">
// CSV import: upload → review the suggested column mapping → dry run → import.
// Nothing is written until the user confirms after the dry run.
import { computed, ref, watch } from 'vue'
import { api, errorMessage, type Schemas } from '@/api/client'
import UiModal from '@/components/ui/UiModal.vue'
import UiButton from '@/components/ui/UiButton.vue'
import { useToast } from '@/composables/useToast'

type Preview = Schemas['CsvPreviewDoc']
type Result = Schemas['ImportResult']

const props = defineProps<{ open: boolean; entityType: 'company' | 'contact' | 'supplier' | 'product'; label: string }>()
const emit = defineEmits<{ close: []; imported: [result: Result] }>()
const toast = useToast()

const step = ref<'file' | 'map' | 'result'>('file')
const file = ref<File | null>(null)
const preview = ref<Preview | null>(null)
const mapping = ref<Record<string, string>>({})
const onDuplicate = ref<'skip' | 'update' | 'create'>('skip')
const dry = ref<Result | null>(null)
const result = ref<Result | null>(null)
const busy = ref(false)
const error = ref('')

watch(
  () => props.open,
  (o) => {
    if (!o) return
    step.value = 'file'
    file.value = null
    preview.value = null
    dry.value = null
    result.value = null
    error.value = ''
    onDuplicate.value = 'skip'
  },
)

const missingRequired = computed(() => {
  if (!preview.value) return []
  const mapped = new Set(Object.values(mapping.value).filter(Boolean))
  return preview.value.fields.filter((f) => f.required && !mapped.has(f.key)).map((f) => f.label)
})

function form(extra: Record<string, string>) {
  const f = new FormData()
  f.append('entity_type', props.entityType)
  for (const [k, v] of Object.entries(extra)) f.append(k, v)
  if (file.value) f.append('file', file.value)
  return f
}

async function loadPreview() {
  if (!file.value) return
  busy.value = true
  error.value = ''
  try {
    preview.value = await api.upload<Preview>('/import/csv/preview', form({}))
    mapping.value = { ...(preview.value.mapping as Record<string, string>) }
    dry.value = null
    step.value = 'map'
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}

function setMapping(header: string, key: string) {
  const next = { ...mapping.value }
  // A field can only be filled from one column.
  if (key) for (const h of Object.keys(next)) if (next[h] === key) next[h] = ''
  next[header] = key
  mapping.value = next
  dry.value = null
}

async function commit(dryRun: boolean) {
  busy.value = true
  error.value = ''
  try {
    const res = await api.upload<Result>('/import/csv/commit', form({ mapping: JSON.stringify(mapping.value), on_duplicate: onDuplicate.value, dry_run: String(dryRun) }))
    if (dryRun) dry.value = res
    else {
      result.value = res
      step.value = 'result'
      emit('imported', res)
      toast.success(`Imported ${res.created} new and ${res.updated} updated ${props.label.toLowerCase()}.`)
    }
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <UiModal :open="open" :title="`Import ${label} from CSV`" size="lg" @close="emit('close')">
    <p v-if="error" class="mb-3 rounded-md bg-red-50 px-3 py-2 text-sm text-red-700" role="alert">{{ error }}</p>

    <div v-if="step === 'file'" class="space-y-3">
      <p class="text-sm text-slate-600">Upload a UTF-8 CSV with a header row. You will review the column mapping and a dry run before anything is saved.</p>
      <input type="file" accept=".csv,text/csv" class="block text-sm" aria-label="CSV file" @change="file = ($event.target as HTMLInputElement).files?.[0] ?? null" />
    </div>

    <div v-else-if="step === 'map' && preview" class="space-y-4">
      <p class="text-sm text-slate-600">{{ preview.total_rows }} rows found. Match each column to a field, or leave it unmapped to ignore it.</p>
      <div class="overflow-x-auto rounded-md border border-slate-200">
        <table class="min-w-full text-sm">
          <thead class="bg-slate-50">
            <tr>
              <th class="table-th">Column</th>
              <th class="table-th">Field</th>
              <th class="table-th">Sample values</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100">
            <tr v-for="(h, i) in preview.headers" :key="h">
              <td class="table-td font-medium">{{ h }}</td>
              <td class="table-td">
                <select class="input" :value="mapping[h] ?? ''" :aria-label="`Field for column ${h}`" @change="setMapping(h, ($event.target as HTMLSelectElement).value)">
                  <option value="">Ignore column</option>
                  <option v-for="f in preview.fields" :key="f.key" :value="f.key">{{ f.label }}{{ f.required ? ' *' : '' }}</option>
                </select>
              </td>
              <td class="table-td max-w-xs truncate text-slate-500">{{ preview.sample_rows.slice(0, 3).map((r) => r[i] ?? '').filter(Boolean).join(' · ') }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <p v-if="missingRequired.length" class="text-sm text-amber-700">Map a column to: {{ missingRequired.join(', ') }}</p>
      <fieldset class="space-y-1">
        <legend class="label">When a row matches an existing record</legend>
        <label class="flex items-center gap-2 text-sm"><input v-model="onDuplicate" type="radio" value="skip" /> Skip the row</label>
        <label class="flex items-center gap-2 text-sm"><input v-model="onDuplicate" type="radio" value="update" /> Update the existing record</label>
        <label class="flex items-center gap-2 text-sm"><input v-model="onDuplicate" type="radio" value="create" /> Create a new record anyway</label>
      </fieldset>
      <div v-if="dry" class="rounded-md border border-slate-200 bg-slate-50 p-3 text-sm" data-testid="dry-run">
        <p class="font-medium text-slate-800">Dry run: {{ dry.created }} to create, {{ dry.updated }} to update, {{ dry.skipped }} skipped, {{ dry.failed }} failed.</p>
        <ul v-if="dry.errors.length" class="mt-2 max-h-40 list-disc overflow-y-auto pl-5 text-red-700">
          <li v-for="e in dry.errors" :key="e.row">Row {{ e.row }}: {{ e.message }}</li>
        </ul>
      </div>
    </div>

    <div v-else-if="step === 'result' && result" class="space-y-2 text-sm">
      <p class="font-medium text-slate-800">Import finished: {{ result.created }} created, {{ result.updated }} updated, {{ result.skipped }} skipped, {{ result.failed }} failed.</p>
      <ul v-if="result.errors.length" class="max-h-48 list-disc overflow-y-auto pl-5 text-red-700">
        <li v-for="e in result.errors" :key="e.row">Row {{ e.row }}: {{ e.message }}</li>
      </ul>
    </div>

    <template #footer>
      <UiButton variant="ghost" @click="emit('close')">{{ step === 'result' ? 'Close' : 'Cancel' }}</UiButton>
      <UiButton v-if="step === 'file'" variant="primary" :disabled="!file" :loading="busy" @click="loadPreview">Continue</UiButton>
      <template v-if="step === 'map'">
        <UiButton :disabled="missingRequired.length > 0" :loading="busy && !dry" @click="commit(true)">Dry run</UiButton>
        <UiButton variant="primary" :disabled="!dry || missingRequired.length > 0" :loading="busy && !!dry" @click="commit(false)">Import {{ preview?.total_rows }} rows</UiButton>
      </template>
    </template>
  </UiModal>
</template>
