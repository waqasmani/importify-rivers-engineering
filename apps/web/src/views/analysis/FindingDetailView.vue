<script setup lang="ts">
// One finding with its classification, confidence, evidence, locator, assumptions, origin and the
// analyzer's structured data. Writers can edit or delete it.
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { Pencil, Sparkles, Trash2 } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import StateViews from '@/components/ui/StateViews.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiButton from '@/components/ui/UiButton.vue'
import ClassificationBadge from '@/components/analysis/ClassificationBadge.vue'
import ConfidenceMeter from '@/components/analysis/ConfidenceMeter.vue'
import FindingFormModal from '@/components/analysis/FindingFormModal.vue'
import VisibleOnlyNotice from '@/components/analysis/VisibleOnlyNotice.vue'
import { CLASSIFICATION_INFO, FINDING_ORIGINS, sourceKindLabel, sourceKindMeta, type Classification } from '@/components/analysis/options'
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
  queryKey: computed(() => ['analysis-finding', props.id]),
  queryFn: ({ signal }) => api.get<Schemas['AnalysisFinding']>(`/analysis/findings/${props.id}`, undefined, signal),
})
const f = computed(() => q.data.value)
const projectId = computed(() => f.value?.project_id ?? '')
const project = useQuery({
  queryKey: computed(() => ['analysis-project', projectId.value]),
  queryFn: ({ signal }) => api.get<Schemas['ProjectDetailDoc']>(`/analysis/projects/${projectId.value}`, undefined, signal),
  enabled: computed(() => !!projectId.value),
})
const sources = useQuery({
  queryKey: computed(() => ['analysis-sources', projectId.value]),
  queryFn: ({ signal }) => api.get<Schemas['ListAnalysisSourcesRowItems']>(`/analysis/projects/${projectId.value}/sources`, undefined, signal),
  enabled: computed(() => !!projectId.value),
})
const source = computed(() => sources.data.value?.items.find((s) => s.id === f.value?.source_id))
const sourceRefs = computed(() => (sources.data.value?.items ?? []).map((s) => ({ id: s.id, title: s.title })))
const origin = computed(() => FINDING_ORIGINS.find((o) => o.value === f.value?.origin))
const dataJson = computed(() => {
  const d = f.value?.data
  if (!d || (typeof d === 'object' && Object.keys(d as object).length === 0)) return ''
  return JSON.stringify(d, null, 2)
})
const cls = computed(() => (f.value?.classification ?? 'unknown') as Classification)
const panel: Record<Classification, string> = {
  verified: 'border-emerald-300 bg-emerald-50/50',
  inferred: 'border-dashed border-amber-400 bg-amber-50/50',
  unknown: 'border-dotted border-slate-400 bg-slate-50',
}
const canWrite = computed(() => auth.can('analysis.write'))

const editing = ref(false)
function onSaved(saved: Schemas['AnalysisFinding']) {
  editing.value = false
  qc.setQueryData(['analysis-finding', props.id], saved)
  void qc.invalidateQueries({ queryKey: ['analysis-findings'] })
  void qc.invalidateQueries({ queryKey: ['analysis-summary', saved.project_id] })
}

async function remove() {
  const x = f.value
  if (!x) return
  const ok = await confirm({ title: 'Delete finding', message: `Delete “${x.title}”? Reports that cite it keep their text but lose the link. This cannot be undone.`, confirmLabel: 'Delete finding', danger: true })
  if (!ok) return
  try {
    await api.del(`/analysis/findings/${x.id}`)
    toast.success('Finding deleted.')
    await qc.invalidateQueries({ queryKey: ['analysis-summary', x.project_id] })
    await qc.invalidateQueries({ queryKey: ['analysis-findings'] })
    await router.push(`/analysis/${x.project_id}?tab=findings`)
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
</script>

<template>
  <div>
    <StateViews v-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <template v-else-if="f">
      <PageHeader :title="f.title" :subtitle="`${f.category.replace(/_/g, ' ')} finding${project.data.value ? ` · ${project.data.value.project.name}` : ''}`" :back="`/analysis/${f.project_id}?tab=findings`">
        <template #meta>
          <div class="mt-2 flex flex-wrap items-center gap-3">
            <ClassificationBadge :value="f.classification" size="md" />
            <ConfidenceMeter :value="f.confidence" wide />
            <UiBadge v-if="f.origin === 'ai'" tone="violet"><Sparkles class="h-3 w-3" /> AI-generated</UiBadge>
          </div>
        </template>
        <template #actions>
          <UiButton v-if="canWrite" @click="editing = true"><Pencil class="h-4 w-4" /> Edit</UiButton>
          <UiButton v-if="canWrite" variant="danger" @click="remove"><Trash2 class="h-4 w-4" /> Delete</UiButton>
        </template>
      </PageHeader>

      <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
        <div class="space-y-4 lg:col-span-2">
          <section class="rounded-lg border p-4" :class="panel[cls]" aria-labelledby="f-class">
            <h2 id="f-class" class="text-sm font-semibold text-slate-800">{{ CLASSIFICATION_INFO[cls].label }}</h2>
            <p class="text-sm text-slate-700">{{ CLASSIFICATION_INFO[cls].description }}</p>
            <p v-if="f.origin === 'ai'" class="mt-2 text-sm text-violet-800">This finding was generated by AI and saved by a user. Verify it against the source before relying on it.</p>
          </section>
          <VisibleOnlyNotice v-if="source && sourceKindMeta(source.kind)?.visibleOnly" compact />
          <section v-if="f.detail" class="card p-4" aria-labelledby="f-detail">
            <h2 id="f-detail" class="mb-1 text-sm font-semibold text-slate-800">Detail</h2>
            <p class="text-sm whitespace-pre-wrap text-slate-800">{{ f.detail }}</p>
          </section>
          <section class="card p-4" aria-labelledby="f-evidence">
            <h2 id="f-evidence" class="mb-1 text-sm font-semibold text-slate-800">Evidence</h2>
            <pre v-if="f.evidence" class="max-h-[28rem] overflow-auto rounded-md bg-slate-50 p-3 font-mono text-xs break-words whitespace-pre-wrap text-slate-800" data-testid="finding-evidence">{{ f.evidence }}</pre>
            <p v-else class="text-sm text-slate-500">{{ f.classification === 'unknown' ? 'No evidence: the sources cannot provide this information.' : 'No evidence text was recorded.' }}</p>
            <p class="mt-2 text-sm"><span class="text-slate-500">Locator:</span> <code v-if="f.evidence_locator" class="text-xs break-all">{{ f.evidence_locator }}</code><span v-else class="text-slate-400">—</span></p>
          </section>
          <section class="card p-4" aria-labelledby="f-assumptions">
            <h2 id="f-assumptions" class="mb-1 text-sm font-semibold text-slate-800">Assumptions</h2>
            <p v-if="f.assumptions" class="text-sm whitespace-pre-wrap text-slate-800">{{ f.assumptions }}</p>
            <p v-else class="text-sm text-slate-500">None stated.</p>
          </section>
          <details v-if="dataJson" class="card p-4">
            <summary class="cursor-pointer text-sm font-semibold text-slate-800">Structured data</summary>
            <pre class="mt-2 max-h-[28rem] overflow-auto rounded-md bg-slate-50 p-3 font-mono text-xs" data-testid="finding-data">{{ dataJson }}</pre>
          </details>
        </div>
        <aside class="card h-fit p-4 text-sm">
          <dl class="grid grid-cols-[6rem_1fr] gap-x-2 gap-y-1.5">
            <dt class="text-slate-500">Origin</dt><dd><span class="font-medium">{{ origin?.label ?? f.origin }}</span><span class="block text-xs text-slate-500">{{ origin?.description }}</span></dd>
            <dt class="text-slate-500">Project</dt><dd><RouterLink :to="`/analysis/${f.project_id}`" class="text-brand-700 hover:underline">{{ project.data.value?.project.name ?? 'Open project' }}</RouterLink></dd>
            <dt class="text-slate-500">Source</dt>
            <dd>
              <RouterLink v-if="f.source_id" :to="`/analysis/sources/${f.source_id}`" class="text-brand-700 hover:underline">{{ source?.title ?? 'Open source' }}</RouterLink>
              <span v-else class="text-slate-500">Not tied to a source</span>
              <span v-if="source" class="block text-xs text-slate-500">{{ sourceKindLabel(source.kind) }}</span>
            </dd>
            <dt class="text-slate-500">Category</dt><dd>{{ f.category.replace(/_/g, ' ') }}</dd>
            <dt class="text-slate-500">Created</dt><dd>{{ formatDateTime(f.created_at) }}</dd>
            <template v-if="f.updated_at !== f.created_at"><dt class="text-slate-500">Edited</dt><dd>{{ formatDateTime(f.updated_at) }}</dd></template>
          </dl>
        </aside>
      </div>

      <FindingFormModal v-if="canWrite" :open="editing" :project-id="f.project_id" :finding="f" :sources="sourceRefs" @close="editing = false" @saved="onSaved" />
    </template>
  </div>
</template>
