<script setup lang="ts">
// New analysis wizard: 1. project, 2. sources, 3. authorization, 4. analyzers.
// Nothing is stored until the user confirms authorization in step 3; sources are then added
// without analysis so that step 4 starts exactly the analyzers the user chose.
import { computed, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useQuery } from '@tanstack/vue-query'
import { Check, Pencil, Play, ShieldCheck, Trash2, TriangleAlert } from 'lucide-vue-next'
import { api, ApiError, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import StateViews from '@/components/ui/StateViews.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import FormField from '@/components/ui/FormField.vue'
import ProjectFields, { type ProjectFieldsModel } from '@/components/analysis/ProjectFields.vue'
import SourceForm from '@/components/analysis/SourceForm.vue'
import VisibleOnlyNotice from '@/components/analysis/VisibleOnlyNotice.vue'
import { useAnalysisConfig } from '@/components/analysis/useAnalysisConfig'
import {
  AUTHORIZATION_BASES, CODE_NOTICE, CREDENTIALS_NOTICE, analyzerLabel, authorizationLabel, defaultAnalyzer, sourceKindLabel, sourceKindMeta,
} from '@/components/analysis/options'
import { draftLabel, emptyDraft, submitSource, validateDraft, type SourceDraft } from '@/components/analysis/sources'
import { useToast } from '@/composables/useToast'
import { formatDateTime } from '@/lib/format'

type Source = Schemas['SourceSummaryDoc']

const route = useRoute()
const router = useRouter()
const toast = useToast()
const config = useAnalysisConfig()

const STEPS = ['Project', 'Sources', 'Authorization', 'Analyzers'] as const
const step = ref(1)

// ---- step 1: project ----
const queryProject = typeof route.query.project === 'string' ? route.query.project : ''
const mode = ref<'new' | 'existing'>(queryProject ? 'existing' : 'new')
const existingId = ref(queryProject)
const createdProjectId = ref<string | null>(null)
const project = ref<ProjectFieldsModel>({ name: '', workspace_id: '', description: '', objective: '', status: 'active', analysis_types: [], owner_id: null })
const projectErrors = ref<Record<string, string>>({})

const workspaces = useQuery({
  queryKey: ['workspaces'],
  queryFn: ({ signal }) => api.get<Schemas['ListWorkspacesRowItems']>('/workspaces', undefined, signal),
  staleTime: 60_000,
})
watch(
  () => workspaces.data.value?.items,
  (items) => {
    if (!project.value.workspace_id && items?.[0]) project.value.workspace_id = items[0].id
  },
  { immediate: true },
)
const projects = useQuery({
  queryKey: ['analysis-projects-picker'],
  queryFn: ({ signal }) => api.get<Schemas['ProjectViewPage']>('/analysis/projects', { limit: 200 }, signal),
  enabled: computed(() => mode.value === 'existing'),
})
const existing = useQuery({
  queryKey: computed(() => ['analysis-project', existingId.value]),
  queryFn: ({ signal }) => api.get<Schemas['ProjectDetailDoc']>(`/analysis/projects/${existingId.value}`, undefined, signal),
  enabled: computed(() => mode.value === 'existing' && !!existingId.value),
})
const existingProject = computed(() => (mode.value === 'existing' ? existing.data.value?.project ?? null : null))
const projectId = computed(() => createdProjectId.value ?? (mode.value === 'existing' ? existingId.value : ''))

function validateProject(): boolean {
  const e: Record<string, string> = {}
  if (mode.value === 'new') {
    if (!project.value.name.trim()) e.name = 'Give the project a name.'
    if (!project.value.workspace_id) e.workspace_id = 'Choose a workspace.'
  } else if (!existingId.value) e.project = 'Choose a project.'
  projectErrors.value = e
  return Object.keys(e).length === 0
}

// ---- step 2: sources ----
interface QueueItem {
  draft: SourceDraft
  state: 'pending' | 'saving' | 'done' | 'error'
  message?: string
  errors?: Record<string, string>
}
const queue = ref<QueueItem[]>([])
const editor = ref<SourceDraft>(emptyDraft())
const editingKey = ref<string | null>(null)
const editorErrors = ref<Record<string, string>>({})
const sourcesError = ref('')

const captureParam = typeof route.query.capture === 'string' ? route.query.capture : ''
let prefilled = false
watch(
  config.sourceKinds,
  (kinds) => {
    if (prefilled || kinds.length === 0) return
    prefilled = true
    if (captureParam && kinds.includes('captured_page')) editor.value = { ...emptyDraft('captured_page'), capturedPageId: captureParam }
  },
  { immediate: true },
)
watch(
  () => editor.value.capturedLabel,
  (label) => {
    if (label && captureParam && mode.value === 'new' && !project.value.name) project.value.name = `Analysis of ${label}`.slice(0, 200)
  },
)

function editorHasInput(d: SourceDraft): boolean {
  return !!(d.title.trim() || d.url.trim() || d.content.trim() || d.capturedPageId || d.path.trim() || d.file || d.steps.some((s) => s.action.trim()))
}
function clearEditorError(f: string) {
  editorErrors.value = Object.fromEntries(Object.entries(editorErrors.value).filter(([k]) => k !== f))
}
function addToQueue(): boolean {
  const e = validateDraft(editor.value, config.maxUploadMb.value)
  editorErrors.value = e
  if (Object.keys(e).length) return false
  const copy: SourceDraft = { ...editor.value, steps: editor.value.steps.map((s) => ({ ...s, data: [...s.data] })) }
  if (editingKey.value) {
    const i = queue.value.findIndex((q) => q.draft.key === editingKey.value)
    if (i >= 0) queue.value[i] = { draft: copy, state: 'pending' }
  } else queue.value.push({ draft: copy, state: 'pending' })
  editingKey.value = null
  editor.value = emptyDraft(copy.kind)
  sourcesError.value = ''
  return true
}
function editItem(item: QueueItem) {
  editingKey.value = item.draft.key
  editor.value = { ...item.draft, steps: item.draft.steps.map((s) => ({ ...s, data: [...s.data] })) }
  editorErrors.value = item.errors ?? {}
  step.value = 2
}
function cancelEdit() {
  editingKey.value = null
  editor.value = emptyDraft(editor.value.kind)
  editorErrors.value = {}
}
function removeItem(item: QueueItem) {
  queue.value = queue.value.filter((q) => q !== item)
  delete confirmations[item.draft.key]
  if (editingKey.value === item.draft.key) cancelEdit()
}
const pending = computed(() => queue.value.filter((q) => q.state !== 'done'))

// ---- step 3: authorization ----
const auth = reactive({ basis: '', note: '', confirmed: false })
const authErrors = ref<Record<string, string>>({})
const confirmations = reactive<Record<string, boolean>>({})
const basisMeta = computed(() => AUTHORIZATION_BASES.find((b) => b.value === auth.basis))
const noteMissing = computed(() => mode.value === 'new' && !createdProjectId.value && !!basisMeta.value?.needsNote && auth.note.trim().length < 10)
const allSourcesConfirmed = computed(() => pending.value.length > 0 && pending.value.every((q) => confirmations[q.draft.key]))
const canSubmit = computed(() => {
  if (!allSourcesConfirmed.value || !auth.confirmed) return false
  if (mode.value === 'new' && !createdProjectId.value) return !!auth.basis && !noteMissing.value
  return true
})
const hasVisibleOnly = computed(() => pending.value.some((q) => sourceKindMeta(q.draft.kind)?.visibleOnly))
const hasCode = computed(() => pending.value.some((q) => sourceKindMeta(q.draft.kind)?.code))
const submitting = ref(false)
const created = ref<Source[]>([])

const PROJECT_FIELDS = ['name', 'workspace_id', 'description', 'objective', 'status', 'analysis_types', 'owner_id']

async function submit() {
  if (!canSubmit.value) return
  submitting.value = true
  authErrors.value = {}
  try {
    if (mode.value === 'new' && !createdProjectId.value) {
      try {
        const p = await api.post<Schemas['AnalysisProject']>('/analysis/projects', {
          ...project.value,
          name: project.value.name.trim(),
          authorization_basis: auth.basis,
          authorization_note: auth.note.trim(),
          authorization_confirmed: auth.confirmed,
        } satisfies Schemas['ProjectInput'])
        createdProjectId.value = p.id
      } catch (e) {
        if (e instanceof ApiError && Object.keys(e.fields).length) {
          const pf = Object.fromEntries(Object.entries(e.fields).filter(([k]) => PROJECT_FIELDS.includes(k)))
          authErrors.value = Object.fromEntries(Object.entries(e.fields).filter(([k]) => !PROJECT_FIELDS.includes(k)))
          if (Object.keys(pf).length) {
            projectErrors.value = pf
            step.value = 1
          }
          toast.error('Please correct the highlighted fields.')
        } else toast.error(errorMessage(e))
        return
      }
    }
    for (const item of queue.value) {
      if (item.state === 'done') continue
      item.state = 'saving'
      item.message = undefined
      item.errors = undefined
      try {
        const doc = await submitSource(projectId.value, item.draft, false)
        item.state = 'done'
        created.value.push(doc.source)
        selection[doc.source.id] = defaultAnalyzer(doc.source.kind) ? [defaultAnalyzer(doc.source.kind)] : []
      } catch (e) {
        item.state = 'error'
        item.message = errorMessage(e)
        if (e instanceof ApiError) item.errors = e.fields
      }
    }
    if (queue.value.every((q) => q.state === 'done')) {
      toast.success(`${created.value.length} source${created.value.length === 1 ? '' : 's'} added.`)
      step.value = 4
    } else toast.error('Some sources could not be added. Fix or remove them and try again.')
  } finally {
    submitting.value = false
  }
}

// ---- step 4: analyzers ----
const selection = reactive<Record<string, string[]>>({})
const runErrors = ref<string[]>([])
const starting = ref(false)
const runCount = computed(() => created.value.reduce((n, s) => n + (selection[s.id]?.length ?? 0), 0))

async function startRuns() {
  starting.value = true
  runErrors.value = []
  let ok = 0
  for (const s of created.value) {
    for (const analyzer of selection[s.id] ?? []) {
      try {
        await api.post<Schemas['AnalysisRun']>(`/analysis/projects/${projectId.value}/runs`, { source_id: s.id, analyzer } satisfies Schemas['RunInput'])
        ok++
      } catch (e) {
        runErrors.value.push(`${s.title} · ${analyzerLabel(analyzer)}: ${errorMessage(e)}`)
      }
    }
  }
  starting.value = false
  if (runErrors.value.length === 0) {
    toast.success(`${ok} analysis run${ok === 1 ? '' : 's'} queued.`)
    await router.push(`/analysis/${projectId.value}?tab=runs`)
  }
}

// ---- navigation ----
function next() {
  if (step.value === 1) {
    if (!validateProject()) return
    step.value = 2
  } else if (step.value === 2) {
    if (editorHasInput(editor.value) && !addToQueue()) return
    if (pending.value.length === 0) {
      sourcesError.value = 'Add at least one source.'
      return
    }
    step.value = 3
  }
}
function goTo(n: number) {
  if (n < step.value && step.value < 4 && !(n === 1 && createdProjectId.value)) step.value = n
}
const title = computed(() => (mode.value === 'existing' && existingProject.value ? `Add sources to ${existingProject.value.name}` : 'New analysis'))
</script>

<template>
  <div class="mx-auto max-w-4xl">
    <PageHeader :title="title" subtitle="Analyze only what you own or are authorized to examine." back="/analysis" />

    <nav aria-label="Wizard steps" class="mb-5">
      <ol class="grid grid-cols-4 gap-1 text-xs sm:text-sm">
        <li v-for="(s, i) in STEPS" :key="s">
          <button
            type="button"
            class="flex w-full flex-col items-start gap-1 border-t-4 pt-2 text-left sm:flex-row sm:items-center"
            :class="i + 1 === step ? 'border-brand-600 font-semibold text-brand-700' : i + 1 < step ? 'border-emerald-500 text-slate-700' : 'border-slate-200 text-slate-400'"
            :aria-current="i + 1 === step ? 'step' : undefined"
            :disabled="!(i + 1 < step && step < 4 && !(i === 0 && createdProjectId))"
            @click="goTo(i + 1)"
          >
            <span class="flex h-5 w-5 items-center justify-center rounded-full text-[11px]" :class="i + 1 < step ? 'bg-emerald-500 text-white' : i + 1 === step ? 'bg-brand-600 text-white' : 'bg-slate-200 text-slate-600'">
              <Check v-if="i + 1 < step" class="h-3 w-3" aria-hidden="true" /><template v-else>{{ i + 1 }}</template>
            </span>
            {{ s }}
          </button>
        </li>
      </ol>
    </nav>

    <StateViews v-if="config.loading.value" kind="loading" />
    <StateViews v-else-if="config.error.value" kind="error" :message="config.error.value" @retry="config.refetch()" />

    <!-- Step 1 -->
    <section v-else-if="step === 1" class="card space-y-4 p-4 sm:p-5" aria-labelledby="step-project">
      <h2 id="step-project" class="text-base font-semibold text-slate-900">Project</h2>
      <fieldset class="flex flex-wrap gap-4 text-sm" :disabled="!!createdProjectId">
        <legend class="sr-only">New or existing project</legend>
        <label class="flex items-center gap-2"><input v-model="mode" type="radio" value="new" /> Create a new project</label>
        <label class="flex items-center gap-2"><input v-model="mode" type="radio" value="existing" /> Add to an existing project</label>
      </fieldset>
      <ProjectFields v-if="mode === 'new'" v-model="project" :errors="projectErrors" />
      <div v-else class="space-y-3">
        <FormField v-slot="{ id, describedby, invalid }" label="Project" required :error="projectErrors.project ?? (projects.error.value ? errorMessage(projects.error.value) : undefined)">
          <select :id="id" v-model="existingId" class="input" :aria-describedby="describedby" :aria-invalid="invalid">
            <option value="" disabled>{{ projects.isLoading.value ? 'Loading…' : 'Choose a project' }}</option>
            <option v-if="existingProject && !(projects.data.value?.items ?? []).some((p) => p.id === existingId)" :value="existingId">{{ existingProject.name }}</option>
            <option v-for="p in projects.data.value?.items ?? []" :key="p.id" :value="p.id">{{ p.name }} ({{ p.workspace_name }})</option>
          </select>
        </FormField>
        <p v-if="existing.error.value" class="text-sm text-red-600" role="alert">{{ errorMessage(existing.error.value) }}</p>
        <dl v-else-if="existingProject" class="grid grid-cols-1 gap-2 rounded-md bg-slate-50 p-3 text-sm sm:grid-cols-2">
          <div><dt class="text-xs text-slate-500">Authorization</dt><dd>{{ authorizationLabel(existingProject.authorization_basis) }}, confirmed {{ formatDateTime(existingProject.authorization_confirmed_at) }}</dd></div>
          <div><dt class="text-xs text-slate-500">Workspace</dt><dd>{{ existing.data.value?.workspace.name }}</dd></div>
          <div v-if="existingProject.objective" class="sm:col-span-2"><dt class="text-xs text-slate-500">Objective</dt><dd class="whitespace-pre-wrap">{{ existingProject.objective }}</dd></div>
        </dl>
      </div>
      <div class="flex justify-end gap-2 border-t border-slate-100 pt-4">
        <RouterLink to="/analysis" class="rounded-md px-3.5 py-2 text-sm font-medium text-slate-600 hover:bg-slate-100">Cancel</RouterLink>
        <UiButton variant="primary" @click="next">Next: sources</UiButton>
      </div>
    </section>

    <!-- Step 2 -->
    <section v-else-if="step === 2" class="space-y-4" aria-labelledby="step-sources">
      <div class="card space-y-4 p-4 sm:p-5">
        <div>
          <h2 id="step-sources" class="text-base font-semibold text-slate-900">{{ editingKey ? 'Edit source' : 'Add sources' }}</h2>
          <p class="text-sm text-slate-600">Add one or more sources. Nothing is uploaded until you confirm authorization in the next step.</p>
          <p v-if="!config.serverFetch.value" class="mt-1 text-xs text-slate-500">Fetching public URLs from the server is turned off by the operator. Capture pages with the browser extension or upload saved HTML instead.</p>
        </div>
        <SourceForm v-model="editor" :kinds="config.sourceKinds.value" :errors="editorErrors" :max-upload-mb="config.maxUploadMb.value" @clear-error="clearEditorError" />
        <div class="flex flex-wrap justify-end gap-2">
          <UiButton v-if="editingKey" variant="ghost" @click="cancelEdit">Cancel edit</UiButton>
          <UiButton data-testid="add-to-list" @click="addToQueue">{{ editingKey ? 'Update source' : 'Add to list' }}</UiButton>
        </div>
      </div>

      <div class="card p-4 sm:p-5">
        <h3 class="text-sm font-semibold text-slate-800">Sources to add ({{ pending.length }})</h3>
        <p v-if="sourcesError" class="mt-1 text-sm text-red-600" role="alert">{{ sourcesError }}</p>
        <p v-if="queue.length === 0" class="mt-2 text-sm text-slate-500">No sources yet. Fill in the form above and choose “Add to list”.</p>
        <ul v-else class="mt-2 divide-y divide-slate-100">
          <li v-for="item in queue" :key="item.draft.key" class="flex items-center gap-2 py-2" data-testid="queued-source">
            <div class="min-w-0 flex-1">
              <p class="truncate text-sm font-medium text-slate-900">{{ draftLabel(item.draft) }}</p>
              <p class="text-xs text-slate-500">{{ sourceKindLabel(item.draft.kind) }}</p>
              <p v-if="item.message" class="text-xs text-red-600" role="alert">{{ item.message }}</p>
            </div>
            <UiBadge v-if="item.state === 'done'" tone="green">Added</UiBadge>
            <template v-else>
              <button type="button" class="rounded p-1.5 text-slate-500 hover:bg-slate-100" :aria-label="`Edit ${draftLabel(item.draft)}`" @click="editItem(item)"><Pencil class="h-4 w-4" /></button>
              <button type="button" class="rounded p-1.5 text-slate-500 hover:bg-red-50 hover:text-red-600" :aria-label="`Remove ${draftLabel(item.draft)}`" @click="removeItem(item)"><Trash2 class="h-4 w-4" /></button>
            </template>
          </li>
        </ul>
        <div class="mt-4 flex justify-between gap-2 border-t border-slate-100 pt-4">
          <UiButton variant="ghost" :disabled="!!createdProjectId" @click="step = 1">Back</UiButton>
          <UiButton variant="primary" @click="next">Next: authorization</UiButton>
        </div>
      </div>
    </section>

    <!-- Step 3 -->
    <section v-else-if="step === 3" class="card space-y-5 p-4 sm:p-5" aria-labelledby="step-auth">
      <div>
        <h2 id="step-auth" class="flex items-center gap-2 text-base font-semibold text-slate-900"><ShieldCheck class="h-5 w-5 text-emerald-700" aria-hidden="true" /> Authorization</h2>
        <div class="mt-2 space-y-2 rounded-md border border-slate-200 bg-slate-50 p-3 text-sm text-slate-700" data-testid="policy">
          <p><strong>REA may only be used on systems and material you own or are authorized to examine.</strong> The basis you confirm is recorded with your name and the time, and it appears on every report.</p>
          <ul class="list-disc space-y-1 pl-5">
            <li>Do not use REA to bypass logins, paywalls, rate limits or other access restrictions, or to copy someone else's product.</li>
            <li>Findings are labelled <em>verified</em> (observed), <em>inferred</em> (reasoned, with assumptions) or <em>unknown</em>. Treat inferences as hypotheses.</li>
            <li>{{ CREDENTIALS_NOTICE }} Recorded API traffic is stripped of headers, cookies and credential-like fields.</li>
            <li>{{ CODE_NOTICE }}</li>
          </ul>
        </div>
      </div>
      <VisibleOnlyNotice v-if="hasVisibleOnly" />
      <p v-if="hasCode" class="rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-900" role="note">You are adding code. {{ CODE_NOTICE }}</p>

      <fieldset v-if="mode === 'new' && !createdProjectId">
        <legend class="label">Basis for this analysis<span class="text-red-600" aria-hidden="true"> *</span></legend>
        <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
          <label v-for="b in AUTHORIZATION_BASES" :key="b.value" class="flex cursor-pointer items-start gap-2 rounded-md border p-2.5 text-sm" :class="auth.basis === b.value ? 'border-brand-500 bg-brand-50' : 'border-slate-200'">
            <input v-model="auth.basis" type="radio" name="authorization-basis" :value="b.value" class="mt-1" />
            <span><span class="block font-medium text-slate-900">{{ b.label }}</span><span class="block text-xs text-slate-500">{{ b.description }}</span></span>
          </label>
        </div>
        <p v-if="authErrors.authorization_basis" class="mt-1 text-xs text-red-600" role="alert">{{ authErrors.authorization_basis }}</p>
      </fieldset>
      <FormField
        v-if="mode === 'new' && !createdProjectId"
        v-slot="{ id, describedby, invalid }"
        label="Authorization note"
        :required="!!basisMeta?.needsNote"
        :hint="basisMeta?.needsNote ? 'Who authorized the analysis, and how (at least 10 characters).' : 'Optional. For example a contract reference.'"
        :error="authErrors.authorization_note"
      >
        <textarea :id="id" v-model="auth.note" rows="2" class="input" :class="{ 'input-invalid': invalid }" maxlength="2000" :aria-describedby="describedby" :aria-invalid="invalid" />
      </FormField>
      <div v-else-if="existingProject || createdProjectId" class="rounded-md bg-slate-50 p-3 text-sm">
        <p class="font-medium text-slate-800">Recorded authorization for this project</p>
        <p v-if="existingProject">{{ authorizationLabel(existingProject.authorization_basis) }} · confirmed {{ formatDateTime(existingProject.authorization_confirmed_at) }}</p>
        <p v-else>{{ authorizationLabel(auth.basis) }} · confirmed just now</p>
        <p v-if="existingProject?.authorization_note" class="mt-1 whitespace-pre-wrap text-slate-600">{{ existingProject.authorization_note }}</p>
      </div>

      <fieldset>
        <legend class="label">Confirm each source<span class="text-red-600" aria-hidden="true"> *</span></legend>
        <ul class="space-y-2">
          <li v-for="item in pending" :key="item.draft.key" class="rounded-md border border-slate-200 p-2.5">
            <label class="flex items-start gap-2 text-sm">
              <input v-model="confirmations[item.draft.key]" type="checkbox" class="mt-0.5 rounded border-slate-300" :data-testid="`confirm-${item.draft.key}`" />
              <span>I own <strong>{{ draftLabel(item.draft) }}</strong> ({{ sourceKindLabel(item.draft.kind).toLowerCase() }}) or am authorized to analyze it.</span>
            </label>
            <p v-if="item.state === 'saving'" class="mt-1 text-xs text-slate-500" role="status">Uploading…</p>
            <div v-if="item.state === 'error'" class="mt-1 flex flex-wrap items-center gap-2 text-xs text-red-700" role="alert">
              <TriangleAlert class="h-3.5 w-3.5" aria-hidden="true" /> {{ item.message }}
              <span v-for="(m, f) in item.errors" :key="f">({{ String(f).replace(/_/g, ' ') }}: {{ m }})</span>
              <UiButton size="sm" variant="ghost" @click="editItem(item)">Fix</UiButton>
              <UiButton size="sm" variant="ghost" @click="removeItem(item)">Remove</UiButton>
            </div>
          </li>
        </ul>
      </fieldset>

      <label class="flex items-start gap-2 rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-950">
        <input v-model="auth.confirmed" type="checkbox" class="mt-0.5 rounded border-slate-300" data-testid="authorization-confirmed" />
        <span>I confirm that I may analyze these sources on the basis stated above, and that I will use the results within that authorization.</span>
      </label>
      <p v-if="authErrors.authorization_confirmed" class="text-xs text-red-600" role="alert">{{ authErrors.authorization_confirmed }}</p>

      <div class="flex flex-wrap justify-between gap-2 border-t border-slate-100 pt-4">
        <UiButton variant="ghost" :disabled="submitting" @click="step = 2">Back</UiButton>
        <UiButton variant="primary" :loading="submitting" :disabled="!canSubmit" data-testid="submit-authorization" @click="submit">
          {{ mode === 'new' && !createdProjectId ? `Create project and add ${pending.length} source${pending.length === 1 ? '' : 's'}` : `Add ${pending.length} source${pending.length === 1 ? '' : 's'}` }}
        </UiButton>
      </div>
    </section>

    <!-- Step 4 -->
    <section v-else class="card space-y-4 p-4 sm:p-5" aria-labelledby="step-analyzers">
      <div>
        <h2 id="step-analyzers" class="text-base font-semibold text-slate-900">Choose analyzers</h2>
        <p class="text-sm text-slate-600">Analyzers run on this server with deterministic rules; no AI is involved. They never execute uploaded code or contact the analyzed system, except fetching a public URL you added.</p>
      </div>
      <ul class="space-y-3">
        <li v-for="s in created" :key="s.id" class="rounded-md border border-slate-200 p-3" :data-testid="`analyzers-${s.id}`">
          <p class="font-medium text-slate-900">{{ s.title }} <span class="text-xs font-normal text-slate-500">· {{ sourceKindLabel(s.kind) }}</span></p>
          <fieldset class="mt-2">
            <legend class="sr-only">Analyzers for {{ s.title }}</legend>
            <div class="flex flex-wrap gap-2">
              <label v-for="a in config.analyzerNames.value" :key="a" class="flex items-center gap-1.5 rounded-full border border-slate-200 px-3 py-1 text-sm has-[:checked]:border-brand-500 has-[:checked]:bg-brand-50">
                <input v-model="selection[s.id]" type="checkbox" :value="a" class="rounded border-slate-300" />
                {{ analyzerLabel(a) }}<span v-if="defaultAnalyzer(s.kind) === a" class="text-xs text-emerald-700">recommended</span>
              </label>
            </div>
          </fieldset>
        </li>
      </ul>
      <ul v-if="runErrors.length" class="list-disc rounded-md bg-red-50 py-2 pr-3 pl-8 text-sm text-red-700" role="alert">
        <li v-for="e in runErrors" :key="e">{{ e }}</li>
      </ul>
      <div class="flex flex-wrap justify-between gap-2 border-t border-slate-100 pt-4">
        <RouterLink :to="`/analysis/${projectId}?tab=sources`" class="rounded-md px-3.5 py-2 text-sm font-medium text-slate-600 hover:bg-slate-100">Open project without running</RouterLink>
        <UiButton variant="primary" :loading="starting" :disabled="runCount === 0" data-testid="start-runs" @click="startRuns"><Play class="h-4 w-4" /> Start {{ runCount }} run{{ runCount === 1 ? '' : 's' }}</UiButton>
      </div>
    </section>
  </div>
</template>
