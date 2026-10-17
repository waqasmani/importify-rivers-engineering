<script setup lang="ts">
// Create or edit a finding. The form enforces the same evidence rules as the server: verified
// findings need evidence, inferred findings need stated assumptions.
import { computed, reactive, ref, watch } from 'vue'
import { api, type Schemas } from '@/api/client'
import UiModal from '@/components/ui/UiModal.vue'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { useToast } from '@/composables/useToast'
import ClassificationBadge from './ClassificationBadge.vue'
import { CLASSIFICATIONS, CLASSIFICATION_INFO } from './options'

type Finding = Schemas['AnalysisFinding']

const props = defineProps<{
  open: boolean
  projectId: string
  /** The finding to edit; omit to create a manual finding. */
  finding?: Finding | null
  sources: { id: string; title: string }[]
  categories?: string[]
  defaultSourceId?: string | null
}>()
const emit = defineEmits<{ close: []; saved: [finding: Finding] }>()
const toast = useToast()
const { errors, capture, clear } = useFieldErrors()
const saving = ref(false)

const form = reactive({
  title: '', category: '', classification: 'verified', confidence: 90, detail: '', evidence: '', evidence_locator: '', assumptions: '', source_id: '' as string,
})

watch(
  () => [props.open, props.finding] as const,
  ([open, f]) => {
    if (!open) return
    clear()
    form.title = f?.title ?? ''
    form.category = f?.category ?? ''
    form.classification = f?.classification ?? 'verified'
    form.confidence = f ? Math.round(Number(f.confidence) * 100) : 90
    form.detail = f?.detail ?? ''
    form.evidence = f?.evidence ?? ''
    form.evidence_locator = f?.evidence_locator ?? ''
    form.assumptions = f?.assumptions ?? ''
    form.source_id = f?.source_id ?? props.defaultSourceId ?? ''
  },
  { immediate: true },
)

const local = computed<Record<string, string>>(() => {
  const e: Record<string, string> = {}
  if (form.classification === 'verified' && !form.evidence.trim()) e.evidence = 'Verified findings need the evidence that was observed.'
  if (form.classification === 'inferred' && !form.assumptions.trim()) e.assumptions = 'Inferred findings need their assumptions.'
  return e
})
const shown = reactive<Record<string, boolean>>({})

async function save() {
  shown.evidence = true
  shown.assumptions = true
  if (Object.keys(local.value).length) return
  const body: Schemas['FindingInput'] = {
    title: form.title,
    category: form.category.trim().toLowerCase().replace(/[\s-]+/g, '_'),
    classification: form.classification,
    confidence: (Math.max(0, Math.min(100, Number(form.confidence) || 0)) / 100).toFixed(3),
    detail: form.detail,
    evidence: form.evidence,
    evidence_locator: form.evidence_locator,
    assumptions: form.assumptions,
    source_id: form.source_id || null,
  }
  saving.value = true
  try {
    const saved = props.finding
      ? await api.put<Finding>(`/analysis/findings/${props.finding.id}`, body)
      : await api.post<Finding>(`/analysis/projects/${props.projectId}/findings`, body)
    toast.success(props.finding ? 'Finding updated.' : 'Finding added.')
    emit('saved', saved)
  } catch (e) {
    capture(e)
  } finally {
    saving.value = false
  }
}
const err = (k: string) => errors.value[k] ?? (shown[k] ? local.value[k] : undefined)
</script>

<template>
  <UiModal :open="open" :title="finding ? 'Edit finding' : 'Add a manual finding'" size="lg" @close="emit('close')">
    <form id="finding-form" class="grid grid-cols-1 gap-4 sm:grid-cols-2" novalidate @submit.prevent="save">
      <FormField v-slot="{ id, describedby, invalid }" label="Title" required :error="err('title')" class="sm:col-span-2">
        <input :id="id" v-model="form.title" class="input" :class="{ 'input-invalid': invalid }" maxlength="500" required :aria-describedby="describedby" :aria-invalid="invalid" />
      </FormField>
      <FormField v-slot="{ id, describedby, invalid }" label="Category" required hint="Lowercase words with underscores, e.g. endpoint, data_model." :error="err('category')">
        <input :id="id" v-model="form.category" class="input" :class="{ 'input-invalid': invalid }" maxlength="41" list="finding-categories" required :aria-describedby="describedby" :aria-invalid="invalid" />
        <datalist id="finding-categories"><option v-for="c in categories ?? []" :key="c" :value="c" /></datalist>
      </FormField>
      <FormField v-slot="{ id, describedby }" label="Source" hint="Optional. The source the evidence comes from." :error="err('source_id')">
        <select :id="id" v-model="form.source_id" class="input" :aria-describedby="describedby">
          <option value="">Not tied to one source</option>
          <option v-for="s in sources" :key="s.id" :value="s.id">{{ s.title }}</option>
        </select>
      </FormField>
      <fieldset class="sm:col-span-2">
        <legend class="label">Classification<span class="text-red-600" aria-hidden="true"> *</span></legend>
        <div class="grid grid-cols-1 gap-2 sm:grid-cols-3">
          <label v-for="c in CLASSIFICATIONS" :key="c" class="flex cursor-pointer items-start gap-2 rounded-md border p-2 text-sm" :class="form.classification === c ? 'border-brand-500 bg-brand-50' : 'border-slate-200'">
            <input v-model="form.classification" type="radio" name="finding-classification" :value="c" class="mt-1" />
            <span><ClassificationBadge :value="c" /><span class="mt-1 block text-xs text-slate-600">{{ CLASSIFICATION_INFO[c].description }}</span></span>
          </label>
        </div>
        <p v-if="err('classification')" class="mt-1 text-xs text-red-600" role="alert">{{ err('classification') }}</p>
      </fieldset>
      <FormField v-slot="{ id, describedby, invalid }" label="Confidence (%)" :error="err('confidence')">
        <input :id="id" v-model.number="form.confidence" type="number" min="0" max="100" step="1" class="input" :class="{ 'input-invalid': invalid }" :aria-describedby="describedby" :aria-invalid="invalid" />
      </FormField>
      <FormField v-slot="{ id, describedby }" label="Evidence locator" hint="Where the evidence is: file:line, URL, selector or page." :error="err('evidence_locator')">
        <input :id="id" v-model="form.evidence_locator" class="input" maxlength="1000" :aria-describedby="describedby" />
      </FormField>
      <FormField v-slot="{ id, describedby, invalid }" label="Evidence" :required="form.classification === 'verified'" hint="What was observed, quoted from the source." :error="err('evidence')" class="sm:col-span-2">
        <textarea :id="id" v-model="form.evidence" rows="4" class="input font-mono text-xs" :class="{ 'input-invalid': invalid }" maxlength="8000" :aria-describedby="describedby" :aria-invalid="invalid" />
      </FormField>
      <FormField v-slot="{ id, describedby, invalid }" label="Assumptions" :required="form.classification === 'inferred'" hint="What must be true for this conclusion to hold." :error="err('assumptions')" class="sm:col-span-2">
        <textarea :id="id" v-model="form.assumptions" rows="2" class="input" :class="{ 'input-invalid': invalid }" :aria-describedby="describedby" :aria-invalid="invalid" />
      </FormField>
      <FormField v-slot="{ id, describedby }" label="Detail" :error="err('detail')" class="sm:col-span-2">
        <textarea :id="id" v-model="form.detail" rows="3" class="input" maxlength="20000" :aria-describedby="describedby" />
      </FormField>
    </form>
    <template #footer>
      <UiButton @click="emit('close')">Cancel</UiButton>
      <UiButton type="submit" form="finding-form" variant="primary" :loading="saving">{{ finding ? 'Save changes' : 'Add finding' }}</UiButton>
    </template>
  </UiModal>
</template>
