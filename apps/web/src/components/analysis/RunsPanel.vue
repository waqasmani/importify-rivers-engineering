<script setup lang="ts">
// Analysis runs: status per analyzer and source, the analyzer's statistics or error, cancelling
// queued runs, and starting a new run for any source.
import { computed, ref, watch } from 'vue'
import { Ban, Play } from 'lucide-vue-next'
import { api, type Schemas } from '@/api/client'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import StateViews from '@/components/ui/StateViews.vue'
import FormField from '@/components/ui/FormField.vue'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { errorMessage } from '@/api/client'
import { formatDateTime, relativeTime } from '@/lib/format'
import { analyzerLabel, defaultAnalyzer, RUN_ACTIVE } from './options'

type Run = Schemas['ListAnalysisRunsRow']

const props = withDefaults(
  defineProps<{
    projectId: string
    runs: Run[]
    loading?: boolean
    error?: string | null
    sources: { id: string; title: string; kind: string }[]
    analyzers: string[]
    canWrite?: boolean
  }>(),
  { loading: false, error: null, canWrite: false },
)
const emit = defineEmits<{ refresh: [] }>()
const toast = useToast()
const { confirm } = useConfirm()
const { errors, capture, clear } = useFieldErrors()

const sourceId = ref('')
const analyzer = ref('')
watch(
  () => props.sources,
  (s) => {
    if (!sourceId.value && s[0]) sourceId.value = s[0].id
  },
  { immediate: true },
)
watch(sourceId, (id) => {
  const s = props.sources.find((x) => x.id === id)
  analyzer.value = s ? defaultAnalyzer(s.kind) : ''
}, { immediate: true })
const starting = ref(false)

async function start() {
  clear()
  starting.value = true
  try {
    await api.post<Schemas['AnalysisRun']>(`/analysis/projects/${props.projectId}/runs`, { source_id: sourceId.value, analyzer: analyzer.value } satisfies Schemas['RunInput'])
    toast.success('Analysis queued. Results appear here as soon as the worker finishes.')
    emit('refresh')
  } catch (e) {
    capture(e)
  } finally {
    starting.value = false
  }
}

async function cancel(r: Run) {
  const ok = await confirm({ title: 'Cancel run', message: `Cancel the queued ${analyzerLabel(r.analyzer)} run for ${r.source_title ?? 'this source'}?`, confirmLabel: 'Cancel run', danger: true })
  if (!ok) return
  try {
    await api.post(`/analysis/runs/${r.id}/cancel`)
    toast.success('Run cancelled.')
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    emit('refresh')
  }
}

function stats(r: Run): [string, string][] {
  const s = r.stats && typeof r.stats === 'object' ? (r.stats as Record<string, unknown>) : {}
  const out: [string, string][] = []
  for (const [k, v] of Object.entries(s)) {
    if (k === 'duration_ms') continue
    if (v && typeof v === 'object') {
      const parts = Object.entries(v as Record<string, unknown>).map(([a, b]) => `${a} ${String(b)}`)
      if (parts.length) out.push([k.replace(/_/g, ' '), parts.join(', ')])
    } else out.push([k.replace(/_/g, ' '), String(v)])
  }
  return out
}
function duration(r: Run): string {
  const s = r.stats && typeof r.stats === 'object' ? (r.stats as Record<string, unknown>).duration_ms : undefined
  if (typeof s === 'number') return s < 1000 ? `${s} ms` : `${(s / 1000).toFixed(1)} s`
  return ''
}
const activeCount = computed(() => props.runs.filter((r) => RUN_ACTIVE.includes(r.status)).length)
</script>

<template>
  <div class="space-y-4">
    <form v-if="canWrite && sources.length" class="card grid grid-cols-1 gap-3 p-4 sm:grid-cols-[1fr_1fr_auto] sm:items-end" @submit.prevent="start">
      <FormField v-slot="{ id }" label="Source" :error="errors.source_id">
        <select :id="id" v-model="sourceId" class="input" required>
          <option v-for="s in sources" :key="s.id" :value="s.id">{{ s.title }}</option>
        </select>
      </FormField>
      <FormField v-slot="{ id }" label="Analyzer" :error="errors.analyzer">
        <select :id="id" v-model="analyzer" class="input" required>
          <option v-for="a in analyzers" :key="a" :value="a">{{ analyzerLabel(a) }}{{ sources.find((s) => s.id === sourceId) && defaultAnalyzer(sources.find((s) => s.id === sourceId)!.kind) === a ? ' (recommended)' : '' }}</option>
        </select>
      </FormField>
      <UiButton type="submit" variant="primary" :loading="starting" :disabled="!sourceId || !analyzer"><Play class="h-4 w-4" /> Start run</UiButton>
      <p class="text-xs text-slate-500 sm:col-span-3">Re-running an analyzer replaces its earlier rule-based findings for that source. Manual and applied AI findings are kept.</p>
    </form>

    <p v-if="activeCount" class="flex items-center gap-2 text-sm text-sky-800" role="status" data-testid="runs-active">
      <span class="h-2 w-2 animate-pulse rounded-full bg-sky-500" aria-hidden="true" /> {{ activeCount }} run{{ activeCount === 1 ? '' : 's' }} in progress. This list refreshes automatically.
    </p>

    <StateViews v-if="loading && runs.length === 0" kind="loading" />
    <StateViews v-else-if="error" kind="error" :message="error" @retry="emit('refresh')" />
    <StateViews v-else-if="runs.length === 0" kind="empty" title="No analysis runs yet" message="Add a source or start a run to analyze it." />
    <ul v-else class="space-y-2">
      <li v-for="r in runs" :key="r.id" class="card p-3" :data-testid="`run-${r.id}`">
        <div class="flex flex-wrap items-center gap-2">
          <UiBadge :value="r.status" :data-testid="`run-status-${r.id}`" />
          <span class="font-medium text-slate-900">{{ analyzerLabel(r.analyzer) }}</span>
          <span class="text-sm text-slate-500">·</span>
          <RouterLink v-if="r.source_id" :to="`/analysis/sources/${r.source_id}`" class="min-w-0 truncate text-sm text-brand-700 hover:underline">{{ r.source_title ?? 'Source' }}</RouterLink>
          <span v-else class="text-sm text-slate-500">Source removed</span>
          <span class="ml-auto text-xs text-slate-500" :title="formatDateTime(r.queued_at)">Queued {{ relativeTime(r.queued_at) }}</span>
          <UiButton v-if="canWrite && r.status === 'queued'" size="sm" variant="ghost" @click="cancel(r)"><Ban class="h-3.5 w-3.5" /> Cancel</UiButton>
        </div>
        <p v-if="r.status === 'running'" class="mt-1 text-xs text-slate-500">Started {{ relativeTime(r.started_at) }}{{ r.attempts > 1 ? ` · attempt ${r.attempts}` : '' }}</p>
        <p v-if="r.error" class="mt-2 rounded-md bg-red-50 px-2 py-1 text-sm text-red-700" role="alert">{{ r.error }}</p>
        <dl v-if="r.status === 'succeeded' && stats(r).length" class="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-slate-600">
          <div v-for="[k, v] in stats(r)" :key="k"><dt class="inline capitalize text-slate-500">{{ k }}:</dt> <dd class="inline font-medium">{{ v }}</dd></div>
          <div v-if="duration(r)"><dt class="inline text-slate-500">took</dt> <dd class="inline font-medium">{{ duration(r) }}</dd></div>
        </dl>
        <p v-if="r.finished_at" class="mt-1 text-xs text-slate-400">Finished {{ formatDateTime(r.finished_at) }}</p>
      </li>
    </ul>
  </div>
</template>
