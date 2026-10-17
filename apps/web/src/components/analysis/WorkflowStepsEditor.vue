<script setup lang="ts">
// Ordered editor for observed workflow steps: actor, action, system, data, outcome and decision.
import { ref, useId, watch } from 'vue'
import { ArrowDown, ArrowUp, Plus, Trash2 } from 'lucide-vue-next'
import UiButton from '@/components/ui/UiButton.vue'
import { splitList } from '@/lib/format'
import { emptyStep, type WorkflowStep } from './sources'

const steps = defineModel<WorkflowStep[]>({ required: true })
defineProps<{ error?: string }>()
const uid = useId()

interface Row {
  actor: string
  action: string
  system: string
  data: string
  outcome: string
  decision: string
}

const rows = ref<Row[]>([])
let emitted: WorkflowStep[] | null = null

function toRow(s: WorkflowStep): Row {
  return { actor: s.actor, action: s.action, system: s.system, data: (s.data ?? []).join(', '), outcome: s.outcome, decision: s.decision }
}

watch(
  steps,
  (v) => {
    if (v === emitted) return
    rows.value = (v ?? []).map(toRow)
  },
  { immediate: true },
)

function sync() {
  emitted = rows.value.map((r) => ({
    actor: r.actor.trim(), action: r.action.trim(), system: r.system.trim(), data: splitList(r.data), outcome: r.outcome.trim(), decision: r.decision.trim(),
  }))
  steps.value = emitted
}

function add() {
  rows.value.push(toRow(emptyStep()))
  sync()
}
function remove(i: number) {
  rows.value.splice(i, 1)
  sync()
}
function move(i: number, d: -1 | 1) {
  const j = i + d
  if (j < 0 || j >= rows.value.length) return
  const r = rows.value.splice(i, 1)[0]!
  rows.value.splice(j, 0, r)
  sync()
}

const fields: { key: keyof Row; label: string; placeholder: string; max: number; wide?: boolean }[] = [
  { key: 'actor', label: 'Actor', placeholder: 'Who acts, e.g. Customer', max: 120 },
  { key: 'action', label: 'Action', placeholder: 'What they do, e.g. Submits an order', max: 500, wide: true },
  { key: 'system', label: 'System', placeholder: 'Screen or system used', max: 120 },
  { key: 'data', label: 'Data', placeholder: 'Comma-separated, e.g. order, address', max: 1000 },
  { key: 'outcome', label: 'Outcome', placeholder: 'What happens next', max: 500 },
  { key: 'decision', label: 'Decision', placeholder: 'Branch or rule, if any', max: 500 },
]
</script>

<template>
  <div>
    <ol class="space-y-3" :aria-describedby="error ? `${uid}-error` : undefined">
      <li v-for="(r, i) in rows" :key="i" class="rounded-md border border-slate-200 bg-slate-50 p-3" data-testid="workflow-step">
        <div class="mb-2 flex items-center justify-between gap-2">
          <span class="text-sm font-semibold text-slate-700">Step {{ i + 1 }}</span>
          <div class="flex gap-1">
            <button type="button" class="rounded p-1 text-slate-500 hover:bg-white disabled:opacity-40" :disabled="i === 0" :aria-label="`Move step ${i + 1} up`" @click="move(i, -1)"><ArrowUp class="h-4 w-4" /></button>
            <button type="button" class="rounded p-1 text-slate-500 hover:bg-white disabled:opacity-40" :disabled="i === rows.length - 1" :aria-label="`Move step ${i + 1} down`" @click="move(i, 1)"><ArrowDown class="h-4 w-4" /></button>
            <button type="button" class="rounded p-1 text-slate-500 hover:bg-red-50 hover:text-red-600 disabled:opacity-40" :disabled="rows.length === 1" :aria-label="`Remove step ${i + 1}`" @click="remove(i)"><Trash2 class="h-4 w-4" /></button>
          </div>
        </div>
        <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
          <div v-for="f in fields" :key="f.key" :class="f.wide ? 'sm:col-span-2' : ''">
            <label :for="`${uid}-${i}-${f.key}`" class="mb-0.5 block text-xs font-medium text-slate-600">{{ f.label }}<span v-if="f.key === 'action'" class="text-red-600" aria-hidden="true"> *</span></label>
            <input :id="`${uid}-${i}-${f.key}`" v-model="r[f.key]" class="input" :maxlength="f.max" :placeholder="f.placeholder" :required="f.key === 'action'" @input="sync" />
          </div>
        </div>
      </li>
    </ol>
    <p v-if="error" :id="`${uid}-error`" class="mt-1 text-xs text-red-600" role="alert">{{ error }}</p>
    <UiButton size="sm" class="mt-2" :disabled="rows.length >= 200" @click="add"><Plus class="h-4 w-4" /> Add step</UiButton>
  </div>
</template>
