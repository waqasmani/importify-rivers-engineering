<script setup lang="ts">
// A report: its Markdown, provenance, an index of the findings it cites, export and delete.
// AI-generated reports carry a prominent banner; deterministic ones say how they were made.
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { Download, FileCheck2, Sparkles, Trash2 } from 'lucide-vue-next'
import { api, download, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import StateViews from '@/components/ui/StateViews.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiButton from '@/components/ui/UiButton.vue'
import MarkdownView from '@/components/content/MarkdownView.vue'
import ClassificationBadge from '@/components/analysis/ClassificationBadge.vue'
import ConfidenceMeter from '@/components/analysis/ConfidenceMeter.vue'
import { CLASSIFICATIONS, CLASSIFICATION_INFO, originLabel, reportKindLabel } from '@/components/analysis/options'
import { useAuth } from '@/stores/auth'
import { useConfirm } from '@/composables/useConfirm'
import { useToast } from '@/composables/useToast'
import { formatDateTime } from '@/lib/format'

const props = defineProps<{ id: string }>()
const router = useRouter()
const auth = useAuth()
const qc = useQueryClient()
const toast = useToast()
const { confirm } = useConfirm()

const q = useQuery({
  queryKey: computed(() => ['analysis-report', props.id]),
  queryFn: ({ signal }) => api.get<Schemas['ReportDetailDoc']>(`/analysis/reports/${props.id}`, undefined, signal),
})
const report = computed(() => q.data.value?.report)
const cited = computed(() => q.data.value?.findings ?? [])
const project = useQuery({
  queryKey: computed(() => ['analysis-project', report.value?.project_id ?? '']),
  queryFn: ({ signal }) => api.get<Schemas['ProjectDetailDoc']>(`/analysis/projects/${report.value!.project_id}`, undefined, signal),
  enabled: computed(() => !!report.value),
})

const ref6 = (id: string) => `F-${id.slice(0, 6)}`
const byClass = computed(() => {
  const out: Record<string, Schemas['AnalysisFinding'][]> = { verified: [], inferred: [], unknown: [] }
  for (const f of cited.value) (out[f.classification] ?? out.unknown!).push(f)
  return out
})
const filter = ref('')
const shown = computed(() => {
  const t = filter.value.trim().toLowerCase()
  return cited.value.filter((f) => !t || ref6(f.id).toLowerCase().includes(t) || f.title.toLowerCase().includes(t))
})

const exporting = ref('')
async function exportAs(format: 'md' | 'json') {
  const r = report.value
  if (!r) return
  exporting.value = format
  try {
    await download(`/analysis/reports/${r.id}/export`, { format }, `report.${format}`)
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    exporting.value = ''
  }
}

async function remove() {
  const r = report.value
  if (!r) return
  const ok = await confirm({ title: 'Delete report', message: `Delete “${r.title}”? The findings it cites are kept. This cannot be undone.`, confirmLabel: 'Delete report', danger: true })
  if (!ok) return
  try {
    await api.del(`/analysis/reports/${r.id}`)
    toast.success('Report deleted.')
    await qc.invalidateQueries({ queryKey: ['analysis-reports', r.project_id] })
    await router.push(`/analysis/${r.project_id}?tab=reports`)
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
</script>

<template>
  <div>
    <StateViews v-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <template v-else-if="report">
      <PageHeader :title="report.title" :subtitle="project.data.value ? project.data.value.project.name : undefined" :back="`/analysis/${report.project_id}?tab=reports`">
        <template #actions>
          <UiButton :loading="exporting === 'md'" @click="exportAs('md')"><Download class="h-4 w-4" /> Markdown</UiButton>
          <UiButton :loading="exporting === 'json'" @click="exportAs('json')"><Download class="h-4 w-4" /> JSON</UiButton>
          <UiButton v-if="auth.can('analysis.write')" variant="danger" @click="remove"><Trash2 class="h-4 w-4" /> Delete</UiButton>
        </template>
      </PageHeader>

      <div v-if="report.is_ai_generated" class="mb-4 flex items-start gap-3 rounded-lg border-2 border-violet-300 bg-violet-50 px-4 py-3 text-violet-950" role="note" data-testid="ai-banner">
        <Sparkles class="mt-0.5 h-5 w-5 shrink-0 text-violet-700" aria-hidden="true" />
        <div>
          <p class="font-semibold">AI-generated — verify against the cited evidence</p>
          <p class="text-sm">Drafted by {{ report.provider_name || 'an AI provider' }} / {{ report.model || 'unknown model' }} and saved by a user. It may contain errors or unsupported statements; check every claim against the findings it cites before relying on it.</p>
        </div>
      </div>
      <div v-else class="mb-4 flex items-start gap-3 rounded-lg border border-emerald-200 bg-emerald-50 px-4 py-3 text-emerald-950" role="note" data-testid="deterministic-banner">
        <FileCheck2 class="mt-0.5 h-5 w-5 shrink-0 text-emerald-700" aria-hidden="true" />
        <div>
          <p class="font-semibold">{{ report.origin === 'manual' ? 'Report written by a person' : 'Deterministic report generated from findings' }}</p>
          <p class="text-sm">{{ report.origin === 'manual' ? 'Content was entered manually.' : 'Assembled by REA’s rules from stored findings, without AI. Each statement cites the finding it comes from.' }}</p>
        </div>
      </div>

      <div class="grid grid-cols-1 gap-6 lg:grid-cols-[1fr_20rem]">
        <article class="card min-w-0 overflow-x-auto p-4 sm:p-6" aria-label="Report content">
          <MarkdownView :source="report.content_markdown" />
        </article>
        <aside class="min-w-0 space-y-4 text-sm">
          <div class="card p-4">
            <h2 class="mb-2 text-sm font-semibold text-slate-800">About this report</h2>
            <dl class="grid grid-cols-[6rem_1fr] gap-x-2 gap-y-1">
              <dt class="text-slate-500">Kind</dt><dd>{{ reportKindLabel(report.kind) }}</dd>
              <dt class="text-slate-500">Origin</dt>
              <dd><UiBadge v-if="report.is_ai_generated" tone="violet"><Sparkles class="h-3 w-3" /> AI-generated</UiBadge><UiBadge v-else :tone="report.origin === 'rule' ? 'green' : 'slate'">{{ report.origin === 'rule' ? 'Deterministic' : report.origin }}</UiBadge></dd>
              <template v-if="report.is_ai_generated">
                <dt class="text-slate-500">Provider</dt><dd>{{ report.provider_name || '—' }}</dd>
                <dt class="text-slate-500">Model</dt><dd class="break-all">{{ report.model || '—' }}</dd>
                <template v-if="report.ai_job_id && auth.can('ai.use')"><dt class="text-slate-500">AI job</dt><dd><RouterLink :to="`/ai/jobs/${report.ai_job_id}`" class="text-brand-700 hover:underline">Open job</RouterLink></dd></template>
              </template>
              <dt class="text-slate-500">Created</dt><dd>{{ formatDateTime(report.created_at) }}</dd>
              <dt class="text-slate-500">Sources</dt><dd>{{ report.source_ids.length }}</dd>
              <dt class="text-slate-500">Citations</dt><dd>{{ report.finding_ids.length }} findings</dd>
            </dl>
          </div>

          <div class="card p-4" data-testid="evidence-index">
            <h2 class="mb-1 text-sm font-semibold text-slate-800">Evidence index</h2>
            <p class="mb-2 text-xs text-slate-500">Citations like [F-1a2b3c] in the report refer to these findings.</p>
            <ul class="mb-2 flex flex-wrap gap-2 text-xs">
              <li v-for="c in CLASSIFICATIONS" :key="c" class="flex items-center gap-1" :title="CLASSIFICATION_INFO[c].description"><ClassificationBadge :value="c" /> {{ byClass[c]?.length ?? 0 }}</li>
            </ul>
            <template v-if="cited.length">
              <label for="evidence-filter" class="sr-only">Filter cited findings</label>
              <input v-if="cited.length > 8" id="evidence-filter" v-model="filter" type="search" class="input mb-2" placeholder="Filter by reference or title" />
              <ul class="max-h-[32rem] space-y-2 overflow-auto">
                <li v-for="f in shown" :key="f.id" class="rounded-md border border-slate-200 p-2" :data-testid="`cited-${f.id}`">
                  <div class="flex flex-wrap items-center gap-1.5">
                    <code class="text-[11px] text-slate-600">[{{ ref6(f.id) }}]</code>
                    <ClassificationBadge :value="f.classification" />
                    <ConfidenceMeter :value="f.confidence" />
                  </div>
                  <RouterLink :to="`/analysis/findings/${f.id}`" class="mt-1 block text-sm font-medium text-brand-700 hover:underline">{{ f.title }}</RouterLink>
                  <p class="text-xs break-all text-slate-500">{{ f.category.replace(/_/g, ' ') }} · {{ originLabel(f.origin) }}<template v-if="f.evidence_locator"> · {{ f.evidence_locator }}</template></p>
                </li>
              </ul>
            </template>
            <p v-else class="text-sm text-slate-500">This report cites no stored findings.</p>
            <p v-if="report.finding_ids.length > cited.length" class="mt-2 text-xs text-amber-700">{{ report.finding_ids.length - cited.length }} cited finding(s) were deleted after the report was created.</p>
          </div>
        </aside>
      </div>
    </template>
  </div>
</template>
