<script setup lang="ts">
// Create or edit a pipeline and its ordered stages. Stages keep their id when edited so records
// stay in them; stages removed here are deleted and their records are left without a stage.
import { computed, reactive, ref, watch } from 'vue'
import { ArrowDown, ArrowUp, Plus, Trash2 } from 'lucide-vue-next'
import { api, type Schemas } from '@/api/client'
import UiModal from '@/components/ui/UiModal.vue'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { useToast } from '@/composables/useToast'
import { titleCase } from '@/lib/format'
import { STAGE_CATEGORIES } from './options'

type Pipeline = Schemas['PipelineView']
interface StageRow {
  uid: number
  id: string | null
  name: string
  category: string
  color: string
  probability: number | '' | null
}

const props = defineProps<{ open: boolean; pipeline: Pipeline | null; entityType: string; entityLabel: string; nextPosition: number }>()
const emit = defineEmits<{ close: []; saved: [pipeline: Pipeline] }>()
const toast = useToast()
const { errors, capture, clear } = useFieldErrors()
const saving = ref(false)
let seq = 0

const form = reactive({ name: '', is_default: false, position: 0, stages: [] as StageRow[] })
const row = (s?: Partial<StageRow>): StageRow => ({ uid: ++seq, id: null, name: '', category: 'open', color: '#64748b', probability: null, ...s })

watch(
  () => props.open,
  (open) => {
    if (!open) return
    clear()
    const p = props.pipeline
    form.name = p?.name ?? ''
    form.is_default = p?.is_default ?? false
    form.position = p?.position ?? props.nextPosition
    form.stages = p
      ? [...p.stages].sort((a, b) => a.position - b.position).map((s) => row({ id: s.id, name: s.name, category: s.category, color: s.color, probability: s.probability ?? null }))
      : [row({ name: 'New', color: '#64748b' }), row({ name: 'In progress', color: '#0ea5e9' }), row({ name: 'Won', category: 'won', color: '#16a34a', probability: 100 }), row({ name: 'Lost', category: 'lost', color: '#dc2626', probability: 0 })]
  },
  { immediate: true },
)

const removedExisting = computed(() => {
  if (!props.pipeline) return []
  const kept = new Set(form.stages.map((s) => s.id).filter(Boolean))
  return props.pipeline.stages.filter((s) => !kept.has(s.id))
})

function move(i: number, d: -1 | 1) {
  const j = i + d
  if (j < 0 || j >= form.stages.length) return
  const s = [...form.stages]
  ;[s[i], s[j]] = [s[j]!, s[i]!]
  form.stages = s
}

async function submit() {
  saving.value = true
  clear()
  const body: Schemas['PipelineInput'] = {
    entity_type: props.pipeline?.entity_type ?? props.entityType,
    name: form.name,
    is_default: form.is_default,
    position: Number(form.position) || 0,
    stages: form.stages.map((s) => ({
      id: s.id,
      name: s.name,
      category: s.category,
      color: s.color,
      probability: s.probability === '' || s.probability === null ? null : Number(s.probability),
    })),
  }
  try {
    const saved = props.pipeline ? await api.put<Pipeline>(`/pipelines/${props.pipeline.id}`, body) : await api.post<Pipeline>('/pipelines', body)
    toast.success(props.pipeline ? 'Pipeline saved.' : 'Pipeline created.')
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
  <UiModal :open="open" :title="pipeline ? `Edit ${pipeline.name}` : `New ${entityLabel.toLowerCase()} pipeline`" size="lg" @close="emit('close')">
    <form id="pipeline-form" class="space-y-4" @submit.prevent="submit">
      <div class="grid grid-cols-1 gap-4 sm:grid-cols-[1fr_8rem]">
        <FormField v-slot="{ id }" label="Name" required :error="errors.name">
          <input :id="id" v-model="form.name" class="input" maxlength="120" required />
        </FormField>
        <FormField v-slot="{ id, describedby }" label="Display order" :error="errors.position" hint="Lower comes first.">
          <input :id="id" v-model.number="form.position" type="number" min="0" step="1" class="input" :aria-describedby="describedby" />
        </FormField>
      </div>
      <label class="flex items-start gap-2 text-sm text-slate-700">
        <input v-model="form.is_default" type="checkbox" class="mt-0.5 rounded border-slate-300" :disabled="pipeline?.is_default" />
        <span>
          Default pipeline for new {{ entityLabel.toLowerCase() }} records
          <span v-if="pipeline?.is_default" class="block text-xs text-slate-500">To change the default, mark another pipeline as default.</span>
        </span>
      </label>

      <fieldset>
        <legend class="label">Stages</legend>
        <p class="mb-2 text-xs text-slate-500">Order stages from first to last. “Won” and “Lost” stages close a record; probability is the expected chance of winning (0–100).</p>
        <ol class="space-y-2">
          <li v-for="(s, i) in form.stages" :key="s.uid" class="grid grid-cols-3 gap-2 rounded-md border border-slate-200 p-2 sm:grid-cols-[2rem_1fr_7rem_6rem_3rem_auto] sm:items-center" data-testid="stage-row">
            <span class="hidden text-center text-xs font-medium text-slate-400 sm:block" aria-hidden="true">{{ i + 1 }}</span>
            <div class="col-span-3 sm:col-span-1">
              <label :for="`stage-name-${s.uid}`" class="sr-only">Stage {{ i + 1 }} name</label>
              <input :id="`stage-name-${s.uid}`" v-model="s.name" class="input" maxlength="80" required placeholder="Stage name" />
            </div>
            <div>
              <label :for="`stage-cat-${s.uid}`" class="sr-only">Stage {{ i + 1 }} category</label>
              <select :id="`stage-cat-${s.uid}`" v-model="s.category" class="input">
                <option v-for="c in STAGE_CATEGORIES" :key="c" :value="c">{{ titleCase(c) }}</option>
              </select>
            </div>
            <div>
              <label :for="`stage-prob-${s.uid}`" class="sr-only">Stage {{ i + 1 }} probability (%)</label>
              <input :id="`stage-prob-${s.uid}`" v-model.number="s.probability" type="number" min="0" max="100" step="1" class="input" placeholder="%" />
            </div>
            <div>
              <label :for="`stage-color-${s.uid}`" class="sr-only">Stage {{ i + 1 }} color</label>
              <input :id="`stage-color-${s.uid}`" v-model="s.color" type="color" class="h-9 w-full cursor-pointer rounded-md border border-slate-300 bg-white p-1" />
            </div>
            <div class="col-span-3 flex justify-end gap-0.5 sm:col-span-1">
              <button type="button" class="rounded p-1.5 text-slate-500 hover:bg-slate-100 disabled:opacity-30" :disabled="i === 0" :aria-label="`Move ${s.name || 'stage'} up`" @click="move(i, -1)"><ArrowUp class="h-4 w-4" /></button>
              <button type="button" class="rounded p-1.5 text-slate-500 hover:bg-slate-100 disabled:opacity-30" :disabled="i === form.stages.length - 1" :aria-label="`Move ${s.name || 'stage'} down`" @click="move(i, 1)"><ArrowDown class="h-4 w-4" /></button>
              <button type="button" class="rounded p-1.5 text-slate-400 hover:bg-red-50 hover:text-red-600 disabled:opacity-30" :disabled="form.stages.length === 1" :aria-label="`Remove ${s.name || 'stage'}`" @click="form.stages.splice(i, 1)"><Trash2 class="h-4 w-4" /></button>
            </div>
          </li>
        </ol>
        <p v-if="errors.stages" class="mt-2 text-xs text-red-600" role="alert">{{ errors.stages }}</p>
        <UiButton size="sm" class="mt-2" :disabled="form.stages.length >= 30" @click="form.stages.push(row())"><Plus class="h-4 w-4" aria-hidden="true" /> Add stage</UiButton>
      </fieldset>

      <p v-if="removedExisting.length" class="rounded-md bg-amber-50 px-3 py-2 text-sm text-amber-800" role="note">
        Removing {{ removedExisting.map((s) => s.name).join(', ') }} leaves any records in {{ removedExisting.length === 1 ? 'that stage' : 'those stages' }} without a stage. They are not deleted.
      </p>
    </form>
    <template #footer>
      <UiButton variant="ghost" @click="emit('close')">Cancel</UiButton>
      <UiButton type="submit" form="pipeline-form" variant="primary" :loading="saving">{{ pipeline ? 'Save pipeline' : 'Create pipeline' }}</UiButton>
    </template>
  </UiModal>
</template>
