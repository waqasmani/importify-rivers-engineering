<script setup lang="ts">
// One analysis source: metadata, the authorization it was added under, a viewer for its kind
// (codebase, API docs, text, website, workflow or screenshot), its findings and AI tools.
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { Download, ExternalLink, Play, ShieldCheck, Trash2 } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import StateViews from '@/components/ui/StateViews.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiButton from '@/components/ui/UiButton.vue'
import AiToolsPanel from '@/components/ai/AiToolsPanel.vue'
import FindingsBrowser from '@/components/analysis/FindingsBrowser.vue'
import ApiViewer from '@/components/analysis/viewers/ApiViewer.vue'
import CodebaseViewer from '@/components/analysis/viewers/CodebaseViewer.vue'
import TextViewer from '@/components/analysis/viewers/TextViewer.vue'
import WebsiteViewer from '@/components/analysis/viewers/WebsiteViewer.vue'
import WorkflowViewer from '@/components/analysis/viewers/WorkflowViewer.vue'
import ScreenshotViewer from '@/components/analysis/viewers/ScreenshotViewer.vue'
import { invalidateProjectResults, useProjectRuns } from '@/components/analysis/useProjectRuns'
import { analyzerLabel, authorizationLabel, defaultAnalyzer, RUN_ACTIVE, sourceKindLabel } from '@/components/analysis/options'
import type { FindingRow } from '@/components/analysis/findingData'
import { useAuth } from '@/stores/auth'
import { useConfirm } from '@/composables/useConfirm'
import { useToast } from '@/composables/useToast'
import { formatBytes, formatDateTime, relativeTime } from '@/lib/format'

const props = defineProps<{ id: string }>()
const router = useRouter()
const auth = useAuth()
const qc = useQueryClient()
const toast = useToast()
const { confirm } = useConfirm()

const q = useQuery({
  queryKey: computed(() => ['analysis-source', props.id]),
  queryFn: ({ signal }) => api.get<Schemas['SourceDetailDoc']>(`/analysis/sources/${props.id}`, undefined, signal),
})
const src = computed(() => q.data.value)
const projectId = computed(() => src.value?.project_id ?? '')
const meta = computed(() => (src.value?.metadata && typeof src.value.metadata === 'object' ? (src.value.metadata as Record<string, unknown>) : {}))

const project = useQuery({
  queryKey: computed(() => ['analysis-project', projectId.value]),
  queryFn: ({ signal }) => api.get<Schemas['ProjectDetailDoc']>(`/analysis/projects/${projectId.value}`, undefined, signal),
  enabled: computed(() => !!projectId.value),
})
const p = computed(() => project.data.value?.project)

// Every finding of this source, for the structured viewers (paged, up to the analyzer maximum).
const findings = useQuery({
  queryKey: computed(() => ['analysis-source-findings', props.id]),
  enabled: computed(() => !!projectId.value),
  queryFn: async ({ signal }) => {
    const out: FindingRow[] = []
    for (let offset = 0; offset < 2400; offset += 200) {
      const page = await api.get<Schemas['FindingListItemPage']>(`/analysis/projects/${projectId.value}/findings`, { source_id: props.id, limit: 200, offset }, signal)
      out.push(...page.items)
      if (out.length >= page.total || page.items.length === 0) break
    }
    return out
  },
})
const findingRows = computed(() => findings.data.value ?? [])

const runs = useProjectRuns(projectId, { onFinished: () => void invalidateProjectResults(qc, projectId.value) })
const myRuns = computed(() => runs.items.value.filter((r) => r.source_id === props.id))
const latest = computed(() => myRuns.value[0])
const lastStats = computed(() => {
  const r = myRuns.value.find((x) => x.status === 'succeeded')
  return r?.stats && typeof r.stats === 'object' ? (r.stats as Record<string, unknown>) : null
})

const directory = useQuery({
  queryKey: ['users-directory'],
  queryFn: ({ signal }) => api.get<Schemas['DirectoryUserItems']>('/users/directory', undefined, signal),
  staleTime: 300_000,
})
const userName = (id?: string | null) => (id ? directory.data.value?.items.find((u) => u.id === id)?.display_name ?? 'Unknown user' : '—')

const viewer = computed(() => {
  switch (src.value?.kind) {
    case 'repository_archive':
    case 'repository_path':
      return 'codebase'
    case 'openapi':
    case 'api_examples':
      return 'api'
    case 'document':
    case 'text':
      return 'text'
    case 'html':
    case 'url':
    case 'captured_page':
      return 'website'
    case 'workflow':
      return 'workflow'
    case 'screenshot':
      return 'screenshot'
  }
  return 'text'
})
const canWrite = computed(() => auth.can('analysis.write'))
const recommended = computed(() => defaultAnalyzer(src.value?.kind ?? ''))
const busy = computed(() => !!latest.value && RUN_ACTIVE.includes(latest.value.status))

async function rerun() {
  if (!src.value) return
  try {
    await api.post<Schemas['AnalysisRun']>(`/analysis/projects/${projectId.value}/runs`, { source_id: src.value.id, analyzer: recommended.value } satisfies Schemas['RunInput'])
    toast.success('Analysis queued.')
    await runs.refetch()
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

async function remove() {
  const s = src.value
  if (!s) return
  const ok = await confirm({
    title: 'Delete source',
    message: `Delete “${s.title}”? Its uploaded file and runs are removed; its findings stay in the project without a source link. This cannot be undone.`,
    confirmLabel: 'Delete source',
    danger: true,
  })
  if (!ok) return
  try {
    await api.del(`/analysis/projects/${s.project_id}/sources/${s.id}`)
    toast.success('Source deleted.')
    await qc.invalidateQueries({ queryKey: ['analysis-sources', s.project_id] })
    await router.push(`/analysis/${s.project_id}?tab=sources`)
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
const isHttp = (u?: string | null) => !!u && /^https?:\/\//i.test(u)
</script>

<template>
  <div>
    <StateViews v-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <template v-else-if="src">
      <PageHeader :title="src.title" :subtitle="`${sourceKindLabel(src.kind)}${p ? ` · ${p.name}` : ''}`" :back="`/analysis/${src.project_id}?tab=sources`">
        <template #meta>
          <div class="mt-2 flex flex-wrap items-center gap-2 text-xs">
            <UiBadge :value="src.status" />
            <template v-if="latest">
              <span class="text-slate-500">Last run:</span>
              <UiBadge :value="latest.status" data-testid="source-run-status" />
              <span class="text-slate-500">{{ analyzerLabel(latest.analyzer) }} · {{ relativeTime(latest.finished_at ?? latest.queued_at) }}</span>
            </template>
          </div>
        </template>
        <template #actions>
          <UiButton v-if="canWrite && recommended" :disabled="busy" @click="rerun"><Play class="h-4 w-4" /> {{ busy ? 'Analysis running…' : 'Run analysis again' }}</UiButton>
          <UiButton v-if="canWrite" variant="danger" @click="remove"><Trash2 class="h-4 w-4" /> Delete</UiButton>
        </template>
      </PageHeader>

      <p v-if="src.error" class="mb-4 rounded-md bg-red-50 px-3 py-2 text-sm text-red-700" role="alert">{{ src.error }}</p>
      <p v-if="latest?.error && latest.error !== src.error" class="mb-4 rounded-md bg-red-50 px-3 py-2 text-sm text-red-700" role="alert">Last run failed: {{ latest.error }}</p>

      <div class="grid grid-cols-1 gap-4 text-sm md:grid-cols-2">
        <div class="card p-4">
          <h2 class="mb-2 text-sm font-semibold text-slate-800">Details</h2>
          <dl class="grid grid-cols-[7rem_1fr] gap-x-2 gap-y-1">
            <dt class="text-slate-500">Kind</dt><dd>{{ sourceKindLabel(src.kind) }}</dd>
            <dt class="text-slate-500">Status</dt><dd>{{ src.status }}</dd>
            <template v-if="src.url"><dt class="text-slate-500">URL</dt><dd class="break-all"><a v-if="isHttp(src.url)" :href="src.url" target="_blank" rel="noopener noreferrer nofollow" class="inline-flex items-center gap-1 text-brand-700 hover:underline">{{ src.url }} <ExternalLink class="h-3 w-3" aria-hidden="true" /></a><span v-else>{{ src.url }}</span></dd></template>
            <template v-if="typeof meta.filename === 'string'"><dt class="text-slate-500">File</dt><dd class="break-words">{{ meta.filename }}</dd></template>
            <template v-if="typeof meta.path === 'string'"><dt class="text-slate-500">Path</dt><dd class="font-mono text-xs break-all">{{ meta.path }}</dd></template>
            <template v-if="src.mime_type"><dt class="text-slate-500">Type</dt><dd>{{ src.mime_type }}</dd></template>
            <template v-if="src.size_bytes"><dt class="text-slate-500">Size</dt><dd>{{ formatBytes(src.size_bytes) }}</dd></template>
            <template v-if="src.content_length"><dt class="text-slate-500">Text</dt><dd>{{ formatBytes(src.content_length) }}</dd></template>
            <template v-if="src.content_sha256"><dt class="text-slate-500">SHA-256</dt><dd class="truncate font-mono text-xs" :title="src.content_sha256">{{ src.content_sha256 }}</dd></template>
            <template v-if="Array.isArray(meta.selections)"><dt class="text-slate-500">Selections</dt><dd>{{ meta.selections.length }} made with the extension</dd></template>
            <dt class="text-slate-500">Added</dt><dd>{{ formatDateTime(src.created_at) }}<br />by {{ userName(src.created_by) }}</dd>
          </dl>
          <a v-if="src.attachment_id" :href="`/api/v1/attachments/${src.attachment_id}/download`" class="mt-3 inline-flex items-center gap-1.5 text-sm text-brand-700 hover:underline"><Download class="h-4 w-4" aria-hidden="true" /> Download the original file</a>
          <p v-if="src.kind === 'api_examples'" class="mt-3 text-xs text-slate-500">Only the sanitized examples are stored: headers, cookies and credential-like fields were removed and the original upload was discarded.</p>
        </div>
        <div class="rounded-md border border-emerald-200 bg-emerald-50/60 p-4" data-testid="source-authorization">
          <h2 class="flex items-center gap-1.5 text-sm font-semibold text-emerald-900"><ShieldCheck class="h-4 w-4" aria-hidden="true" /> Authorization</h2>
          <template v-if="p">
            <p class="mt-1 text-slate-700">Added under the project's recorded basis: <strong>{{ authorizationLabel(p.authorization_basis) }}</strong>, confirmed {{ formatDateTime(p.authorization_confirmed_at) }} by {{ userName(p.authorization_confirmed_by) }}.</p>
            <p v-if="p.authorization_note" class="mt-1 whitespace-pre-wrap text-slate-600">{{ p.authorization_note }}</p>
            <RouterLink :to="`/analysis/${p.id}`" class="mt-2 inline-block text-brand-700 hover:underline">Open project</RouterLink>
          </template>
          <p v-else-if="project.error.value" class="mt-1 text-red-700">{{ errorMessage(project.error.value) }}</p>
          <p v-else class="mt-1 text-slate-500">Loading…</p>
        </div>
      </div>

      <section class="card mt-6 min-w-0 p-4" aria-labelledby="source-contents">
        <h2 id="source-contents" class="mb-3 text-sm font-semibold text-slate-800">Contents</h2>
        <StateViews v-if="findings.isLoading.value" kind="loading" />
        <StateViews v-else-if="findings.error.value" kind="error" :message="errorMessage(findings.error.value)" @retry="findings.refetch()" />
        <template v-else>
          <CodebaseViewer v-if="viewer === 'codebase'" :findings="findingRows" :stats="lastStats" :metadata="meta" />
          <template v-else-if="viewer === 'api'">
            <ApiViewer :kind="src.kind" :findings="findingRows" />
            <details class="mt-5">
              <summary class="cursor-pointer text-sm font-medium text-slate-700">{{ src.kind === 'openapi' ? 'API description source' : 'Stored examples (sanitized)' }}</summary>
              <div class="mt-2"><TextViewer :text="src.content_text" :truncated="src.content_truncated" :total-length="src.content_length" :kind="src.kind" :findings="[]" /></div>
            </details>
          </template>
          <TextViewer v-else-if="viewer === 'text'" :text="src.content_text" :truncated="src.content_truncated" :total-length="src.content_length" :kind="src.kind" :findings="findingRows" />
          <template v-else-if="viewer === 'website'">
            <WebsiteViewer :url="src.url" :metadata="meta" :captured-page-id="src.captured_page_id" :findings="findingRows" :can-see-captures="auth.can('products.read')" />
            <details v-if="src.content_text" class="mt-5">
              <summary class="cursor-pointer text-sm font-medium text-slate-700">Page source</summary>
              <div class="mt-2"><TextViewer :text="src.content_text" :truncated="src.content_truncated" :total-length="src.content_length" kind="html" :findings="[]" /></div>
            </details>
          </template>
          <WorkflowViewer v-else-if="viewer === 'workflow'" :metadata="meta" :text="src.content_text" :findings="findingRows" />
          <ScreenshotViewer v-else :attachment-id="src.attachment_id" :title="src.title" :findings="findingRows" />
        </template>
      </section>


      <section class="card mt-6 p-4" aria-labelledby="source-findings">
        <h2 id="source-findings" class="mb-3 text-sm font-semibold text-slate-800">Findings from this source</h2>
        <FindingsBrowser
          v-if="projectId"
          :project-id="projectId"
          :source-id="src.id"
          :sources="[{ id: src.id, title: src.title }]"
          :categories="[...new Set(findingRows.map((f) => f.category))].sort()"
          :can-write="canWrite"
          @changed="findings.refetch()"
        />
      </section>

      <section v-if="auth.can('ai.use')" class="card mt-6 p-4" aria-labelledby="source-ai">
        <h2 id="source-ai" class="mb-1 text-sm font-semibold text-slate-800">AI tools</h2>
        <p class="mb-3 text-xs text-slate-500">Summaries, structured extraction and explanations use only this source's stored evidence. Results are labelled as AI-generated and change nothing until you apply them.</p>
        <AiToolsPanel entity-type="analysis_source" :entity-id="src.id" @applied="invalidateProjectResults(qc, projectId)" />
      </section>
    </template>
  </div>
</template>
