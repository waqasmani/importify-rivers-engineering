<script setup lang="ts">
// One analysis project: overview of findings by category and classification, sources, findings,
// runs (polled while active), reports, AI drafting tools, and notes/files/activity.
import { computed, ref, toRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { Pencil, Plus, ShieldCheck, Sparkles, Trash2 } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import StateViews from '@/components/ui/StateViews.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiModal from '@/components/ui/UiModal.vue'
import UiTabs from '@/components/ui/UiTabs.vue'
import TagEditor from '@/components/record/TagEditor.vue'
import RecordPanels from '@/components/record/RecordPanels.vue'
import AiToolsPanel from '@/components/ai/AiToolsPanel.vue'
import ProjectFields, { type ProjectFieldsModel } from '@/components/analysis/ProjectFields.vue'
import FindingSummary from '@/components/analysis/FindingSummary.vue'
import FindingsBrowser from '@/components/analysis/FindingsBrowser.vue'
import SourcesPanel from '@/components/analysis/SourcesPanel.vue'
import RunsPanel from '@/components/analysis/RunsPanel.vue'
import ReportsPanel from '@/components/analysis/ReportsPanel.vue'
import VisibleOnlyNotice from '@/components/analysis/VisibleOnlyNotice.vue'
import { useAnalysisConfig } from '@/components/analysis/useAnalysisConfig'
import { invalidateProjectResults, useProjectRuns } from '@/components/analysis/useProjectRuns'
import { ANALYSIS_TYPES, authorizationLabel, sourceKindMeta } from '@/components/analysis/options'
import { useAuth } from '@/stores/auth'
import { useConfirm } from '@/composables/useConfirm'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { useToast } from '@/composables/useToast'
import { formatDateTime, relativeTime } from '@/lib/format'

const props = defineProps<{ id: string }>()
const route = useRoute()
const router = useRouter()
const auth = useAuth()
const qc = useQueryClient()
const toast = useToast()
const { confirm } = useConfirm()
const config = useAnalysisConfig()
const idRef = toRef(props, 'id')

const q = useQuery({
  queryKey: computed(() => ['analysis-project', props.id]),
  queryFn: ({ signal }) => api.get<Schemas['ProjectDetailDoc']>(`/analysis/projects/${props.id}`, undefined, signal),
})
const doc = computed(() => q.data.value)
const project = computed(() => doc.value?.project)

const summary = useQuery({
  queryKey: computed(() => ['analysis-summary', props.id]),
  queryFn: ({ signal }) => api.get<Schemas['FindingCategoryCountsRowItems']>(`/analysis/projects/${props.id}/findings/summary`, undefined, signal),
})
const summaryRows = computed(() => summary.data.value?.items ?? [])
const findingTotal = computed(() => summaryRows.value.reduce((n, r) => n + r.finding_count, 0))
const categories = computed(() => [...new Set(summaryRows.value.map((r) => r.category))].sort())

const sources = useQuery({
  queryKey: computed(() => ['analysis-sources', props.id]),
  queryFn: ({ signal }) => api.get<Schemas['ListAnalysisSourcesRowItems']>(`/analysis/projects/${props.id}/sources`, undefined, signal),
})
const sourceItems = computed(() => sources.data.value?.items ?? [])
const sourceRefs = computed(() => sourceItems.value.map((s) => ({ id: s.id, title: s.title, kind: s.kind })))
const hasVisibleOnly = computed(() => sourceItems.value.some((s) => sourceKindMeta(s.kind)?.visibleOnly))

const runs = useProjectRuns(idRef, {
  onFinished: (done) => {
    void invalidateProjectResults(qc, props.id)
    const failed = done.filter((r) => r.status === 'failed').length
    if (failed) toast.error(`${failed} analysis run${failed === 1 ? '' : 's'} failed. See the Runs tab.`)
    else toast.info(`${done.length} analysis run${done.length === 1 ? '' : 's'} finished.`)
  },
})

const directory = useQuery({
  queryKey: ['users-directory'],
  queryFn: ({ signal }) => api.get<Schemas['DirectoryUserItems']>('/users/directory', undefined, signal),
  staleTime: 300_000,
})
const userName = (id?: string | null) => (id ? directory.data.value?.items.find((u) => u.id === id)?.display_name ?? 'Unknown user' : '—')

const canWrite = computed(() => auth.can('analysis.write'))
const tabs = computed(() => [
  { key: 'overview', label: 'Overview' },
  { key: 'sources', label: 'Sources', count: sourceItems.value.length },
  { key: 'findings', label: 'Findings', count: findingTotal.value },
  { key: 'runs', label: 'Runs', count: runs.items.value.length },
  { key: 'reports', label: 'Reports' },
  ...(auth.can('ai.use') ? [{ key: 'ai', label: 'AI' }] : []),
])
const tab = computed({
  get: () => {
    const t = typeof route.query.tab === 'string' ? route.query.tab : 'overview'
    return tabs.value.some((x) => x.key === t) ? t : 'overview'
  },
  set: (t: string) => void router.replace({ query: { ...route.query, tab: t } }),
})
function openFindings(f: { category?: string; classification?: string }) {
  void router.replace({ query: { tab: 'findings', ...(f.category ? { category: f.category } : {}), ...(f.classification ? { classification: f.classification } : {}) } })
}
function refreshSources() {
  void sources.refetch()
  void q.refetch()
  void runs.refetch()
}

// ---- edit ----
const editing = ref(false)
const form = ref<ProjectFieldsModel>({ name: '', workspace_id: '', description: '', objective: '', status: 'active', analysis_types: [], owner_id: null })
const { errors, capture, clear } = useFieldErrors()
const saving = ref(false)
function startEdit() {
  const p = project.value
  if (!p) return
  clear()
  form.value = { name: p.name, workspace_id: p.workspace_id, description: p.description, objective: p.objective, status: p.status, analysis_types: [...p.analysis_types], owner_id: p.owner_id }
  editing.value = true
}
async function save() {
  const p = project.value
  if (!p) return
  saving.value = true
  try {
    await api.put<Schemas['AnalysisProject']>(`/analysis/projects/${p.id}`, {
      ...form.value,
      authorization_basis: p.authorization_basis,
      authorization_note: p.authorization_note,
      authorization_confirmed: true,
    } satisfies Schemas['ProjectInput'])
    toast.success('Project updated.')
    editing.value = false
    await q.refetch()
  } catch (e) {
    capture(e)
  } finally {
    saving.value = false
  }
}

async function remove() {
  const p = project.value
  if (!p) return
  const ok = await confirm({
    title: 'Delete analysis project',
    message: `Delete “${p.name}” with all of its sources, runs, findings and reports? This cannot be undone.`,
    details: [`${sourceItems.value.length} sources`, `${findingTotal.value} findings`],
    confirmLabel: 'Delete project',
    danger: true,
  })
  if (!ok) return
  try {
    await api.del(`/analysis/projects/${p.id}`)
    toast.success(`Deleted ${p.name}.`)
    await qc.invalidateQueries({ queryKey: ['analysis-projects'] })
    await router.push('/analysis')
  } catch (e) {
    toast.error(errorMessage(e))
  }
}

const tags = ref<Schemas['TagRef'][]>([])
watch(() => doc.value?.tags, (t) => (tags.value = t ?? []), { immediate: true })
const typeLabel = (t: string) => ANALYSIS_TYPES.find((x) => x.value === t)?.label ?? t
</script>

<template>
  <div>
    <StateViews v-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <template v-else-if="project && doc">
      <PageHeader :title="project.name" :subtitle="`${doc.workspace.name} · updated ${relativeTime(project.updated_at)}`" back="/analysis">
        <template #meta>
          <div class="mt-2 flex flex-wrap items-center gap-2 text-xs text-slate-600">
            <UiBadge :value="project.status" />
            <span class="inline-flex items-center gap-1" :title="`Confirmed ${formatDateTime(project.authorization_confirmed_at)} by ${userName(project.authorization_confirmed_by)}`">
              <ShieldCheck class="h-3.5 w-3.5 text-emerald-600" aria-hidden="true" /> {{ authorizationLabel(project.authorization_basis) }}
            </span>
            <UiBadge v-for="t in project.analysis_types" :key="t" tone="blue">{{ typeLabel(t) }}</UiBadge>
            <TagEditor entity-type="analysis_project" :entity-id="project.id" :tags="tags" :can-edit="canWrite" @updated="(t) => (tags = t)" />
          </div>
        </template>
        <template #actions>
          <RouterLink v-if="canWrite" :to="`/analysis/new?project=${project.id}`" class="inline-flex items-center gap-1.5 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700"><Plus class="h-4 w-4" aria-hidden="true" /> Add sources</RouterLink>
          <UiButton v-if="canWrite" @click="startEdit"><Pencil class="h-4 w-4" /> Edit</UiButton>
          <UiButton v-if="auth.can('analysis.delete')" variant="danger" @click="remove"><Trash2 class="h-4 w-4" /> Delete</UiButton>
        </template>
      </PageHeader>

      <p v-if="runs.active.value.length" class="mb-4 flex items-center gap-2 rounded-md bg-sky-50 px-3 py-2 text-sm text-sky-900" role="status">
        <span class="h-2 w-2 animate-pulse rounded-full bg-sky-500" aria-hidden="true" />
        {{ runs.active.value.length }} analysis run{{ runs.active.value.length === 1 ? ' is' : 's are' }} in progress. Findings update automatically.
      </p>

      <section class="card mb-6 p-4" aria-label="Project sections">
        <UiTabs v-model="tab" :tabs="tabs">
          <div v-if="tab === 'overview'" class="grid grid-cols-1 gap-6 lg:grid-cols-3">
            <div class="min-w-0 space-y-4 lg:col-span-2">
              <div>
                <h2 class="mb-2 text-sm font-semibold text-slate-800">Findings by category and classification</h2>
                <StateViews v-if="summary.isLoading.value" kind="loading" />
                <StateViews v-else-if="summary.error.value" kind="error" :message="errorMessage(summary.error.value)" @retry="summary.refetch()" />
                <StateViews v-else-if="summaryRows.length === 0" kind="empty" title="No findings yet" :message="sourceItems.length ? 'Findings appear when analysis runs finish.' : 'Add a source to start the analysis.'">
                  <RouterLink v-if="canWrite && sourceItems.length === 0" :to="`/analysis/new?project=${project.id}`" class="mt-2 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700">Add sources</RouterLink>
                </StateViews>
                <FindingSummary v-else :rows="summaryRows" @open="openFindings" />
              </div>
              <VisibleOnlyNotice v-if="hasVisibleOnly" compact />
            </div>
            <aside class="min-w-0 space-y-4 text-sm">
              <div class="rounded-md border border-emerald-200 bg-emerald-50/60 p-3" data-testid="authorization-record">
                <h2 class="flex items-center gap-1.5 font-semibold text-emerald-900"><ShieldCheck class="h-4 w-4" aria-hidden="true" /> Authorization record</h2>
                <dl class="mt-2 space-y-1 text-slate-700">
                  <div><dt class="inline text-slate-500">Basis:</dt> <dd class="inline">{{ authorizationLabel(project.authorization_basis) }}</dd></div>
                  <div><dt class="inline text-slate-500">Confirmed by:</dt> <dd class="inline">{{ userName(project.authorization_confirmed_by) }}</dd></div>
                  <div><dt class="inline text-slate-500">Confirmed at:</dt> <dd class="inline">{{ formatDateTime(project.authorization_confirmed_at) }}</dd></div>
                  <div v-if="project.authorization_note"><dt class="text-slate-500">Note:</dt> <dd class="whitespace-pre-wrap">{{ project.authorization_note }}</dd></div>
                </dl>
              </div>
              <dl class="space-y-2">
                <div v-if="project.objective"><dt class="text-xs text-slate-500">Objective</dt><dd class="whitespace-pre-wrap text-slate-800">{{ project.objective }}</dd></div>
                <div v-if="project.description"><dt class="text-xs text-slate-500">Description</dt><dd class="whitespace-pre-wrap text-slate-800">{{ project.description }}</dd></div>
                <div><dt class="text-xs text-slate-500">Owner</dt><dd>{{ userName(project.owner_id) }}</dd></div>
                <div><dt class="text-xs text-slate-500">Created</dt><dd>{{ formatDateTime(project.created_at) }} by {{ userName(project.created_by) }}</dd></div>
              </dl>
            </aside>
          </div>

          <SourcesPanel
            v-else-if="tab === 'sources'"
            :project="project"
            :sources="sourceItems"
            :loading="sources.isLoading.value"
            :error="sources.error.value ? errorMessage(sources.error.value) : null"
            :kinds="config.sourceKinds.value"
            :max-upload-mb="config.maxUploadMb.value"
            :can-write="canWrite"
            @changed="refreshSources"
          />

          <FindingsBrowser v-else-if="tab === 'findings'" :project-id="project.id" :sources="sourceRefs" :categories="categories" :can-write="canWrite" @changed="summary.refetch()" />

          <RunsPanel
            v-else-if="tab === 'runs'"
            :project-id="project.id"
            :runs="runs.items.value"
            :loading="runs.loading.value"
            :error="runs.error.value"
            :sources="sourceRefs"
            :analyzers="config.analyzerNames.value"
            :can-write="canWrite"
            @refresh="runs.refetch()"
          />

          <ReportsPanel v-else-if="tab === 'reports'" :project-id="project.id" :report-kinds="config.reportKinds.value" :can-write="canWrite" :finding-count="findingTotal" />

          <div v-else-if="tab === 'ai'" class="space-y-4">
            <div class="rounded-md border border-violet-200 bg-violet-50 px-3 py-2 text-sm text-violet-900" role="note">
              <p class="flex items-center gap-1.5 font-medium"><Sparkles class="h-4 w-4" aria-hidden="true" /> AI drafting tools</p>
              <p class="mt-1">
                Draft technical documentation, an architecture explanation, a data model, an API integration plan or a reconstruction plan from this project's findings.
                Every result is labelled as AI-generated, cites the findings it used and changes nothing until you apply it as a report. Deterministic reports in the Reports tab need no AI.
              </p>
            </div>
            <AiToolsPanel entity-type="analysis_project" :entity-id="project.id" @applied="qc.invalidateQueries({ queryKey: ['analysis-reports', project.id] })" />
          </div>
        </UiTabs>
      </section>

      <RecordPanels entity-type="analysis_project" :entity-id="project.id" :ai="false" />

      <UiModal :open="editing" title="Edit project" size="lg" @close="editing = false">
        <form id="project-edit-form" @submit.prevent="save">
          <ProjectFields v-model="form" :errors="errors" />
          <p class="mt-3 text-xs text-slate-500">The authorization basis is recorded when the project is created and cannot be changed. Create a new project if the basis changes.</p>
        </form>
        <template #footer>
          <UiButton @click="editing = false">Cancel</UiButton>
          <UiButton type="submit" form="project-edit-form" variant="primary" :loading="saving">Save changes</UiButton>
        </template>
      </UiModal>
    </template>
  </div>
</template>
