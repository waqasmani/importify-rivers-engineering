<script setup lang="ts">
// Adds one source to an existing project after the user confirms they may analyze it.
import { computed, reactive, ref, watch } from 'vue'
import { ShieldCheck } from 'lucide-vue-next'
import type { Schemas } from '@/api/client'
import UiModal from '@/components/ui/UiModal.vue'
import UiButton from '@/components/ui/UiButton.vue'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { useToast } from '@/composables/useToast'
import SourceForm from './SourceForm.vue'
import { analyzerLabel, authorizationLabel, defaultAnalyzer } from './options'
import { draftLabel, emptyDraft, submitSource, validateDraft, type SourceDraft } from './sources'

const props = defineProps<{ open: boolean; project: Schemas['AnalysisProject']; kinds: string[]; maxUploadMb: number }>()
const emit = defineEmits<{ close: []; added: [doc: Schemas['SourceCreatedDoc']] }>()
const toast = useToast()
const { errors, capture, clear } = useFieldErrors()

const draft = ref<SourceDraft>(emptyDraft())
const confirmed = ref(false)
const analyze = ref(true)
const saving = ref(false)
const local = reactive<Record<string, string>>({})
watch(
  () => props.open,
  (o) => {
    if (!o) return
    draft.value = emptyDraft()
    confirmed.value = false
    analyze.value = true
    clear()
    for (const k of Object.keys(local)) delete local[k]
  },
  { immediate: true },
)
const shownErrors = computed(() => ({ ...local, ...errors.value }))
function clearError(f: string) {
  delete local[f]
  if (errors.value[f]) errors.value = Object.fromEntries(Object.entries(errors.value).filter(([k]) => k !== f))
}
const recommended = computed(() => defaultAnalyzer(draft.value.kind))

async function save() {
  clear()
  for (const k of Object.keys(local)) delete local[k]
  Object.assign(local, validateDraft(draft.value, props.maxUploadMb))
  if (Object.keys(local).length || !confirmed.value) return
  saving.value = true
  try {
    const doc = await submitSource(props.project.id, draft.value, analyze.value)
    toast.success(doc.run ? `Added ${draftLabel(draft.value)}. Analysis queued.` : `Added ${draftLabel(draft.value)}.`)
    emit('added', doc)
  } catch (e) {
    capture(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <UiModal :open="open" title="Add a source" size="xl" @close="emit('close')">
    <form id="add-source-form" class="space-y-4" novalidate @submit.prevent="save">
      <SourceForm v-model="draft" :kinds="kinds" :errors="shownErrors" :max-upload-mb="maxUploadMb" @clear-error="clearError" />
      <div class="space-y-2 rounded-md border border-slate-200 bg-slate-50 p-3">
        <label class="flex items-start gap-2 text-sm text-slate-800">
          <input v-model="confirmed" type="checkbox" class="mt-0.5 rounded border-slate-300" data-testid="source-authorized" />
          <span>
            <ShieldCheck class="mr-1 inline h-4 w-4 text-emerald-700" aria-hidden="true" />
            I own this material or am authorized to analyze it, under this project's recorded basis: <strong>{{ authorizationLabel(project.authorization_basis) }}</strong>.
          </span>
        </label>
        <label v-if="recommended" class="flex items-center gap-2 text-sm text-slate-700">
          <input v-model="analyze" type="checkbox" class="rounded border-slate-300" /> Analyze it now with the {{ analyzerLabel(recommended) }} analyzer
        </label>
      </div>
    </form>
    <template #footer>
      <UiButton @click="emit('close')">Cancel</UiButton>
      <UiButton type="submit" form="add-source-form" variant="primary" :loading="saving" :disabled="!confirmed || !draft.kind">Add source</UiButton>
    </template>
  </UiModal>
</template>
