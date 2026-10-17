<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { Sparkles } from 'lucide-vue-next'
import type { SummarizeResult } from '../../shared/contract'
import type { UiError } from '../../shared/messages'
import type { Schemas } from '../../shared/types'
import { asUiError, call } from '../bridge'
import { app, can, link, saveCapture } from '../store'
import ErrorNote from './ErrorNote.vue'
import ExtLink from './ExtLink.vue'
import Notice from './Notice.vue'
import SectionCard from './SectionCard.vue'
import Spinner from './Spinner.vue'

const BASES = [
  { value: 'public_information', label: 'Public information' },
  { value: 'owned', label: 'We own this site' },
  { value: 'client_authorized', label: 'A client authorized it' },
  { value: 'licensed', label: 'Licensed material' },
  { value: 'open_source', label: 'Open source' },
  { value: 'other', label: 'Other' },
] as const

const connected = computed(() => !!app.status?.connected)
const aiEnabled = computed(() => !!app.status?.config?.ai_enabled)
const capture = computed(() => app.capture)

// ---- analysis project ----
const workspaces = ref<Schemas['ListWorkspacesRow'][]>([])
const project = reactive({
  workspace_id: '',
  name: '',
  basis: 'public_information' as (typeof BASES)[number]['value'],
  note: '',
  confirmed: false,
})
const creating = ref(false)
const projectError = ref<UiError | null>(null)
const created = ref<{ project: Schemas['AnalysisProject']; source: Schemas['SourceCreatedDoc'] } | null>(null)
const noteRequired = computed(() => project.basis === 'other' || project.basis === 'client_authorized')

watch(
  () => app.extraction?.domain,
  (d) => { if (d && !project.name) project.name = `Analysis of ${d}` },
  { immediate: true },
)

async function loadWorkspaces(): Promise<void> {
  if (!connected.value || !can('analysis.read')) return
  try {
    workspaces.value = await call({ type: 'analysis.workspaces' })
    if (!project.workspace_id && workspaces.value[0]) project.workspace_id = workspaces.value[0].id
  } catch (e) {
    projectError.value = asUiError(e)
  }
}
void loadWorkspaces()

async function createProject(): Promise<void> {
  if (!capture.value) return
  creating.value = true
  projectError.value = null
  try {
    created.value = await call({
      type: 'analysis.create',
      capturedPageId: capture.value.id,
      project: {
        workspace_id: project.workspace_id,
        name: project.name.trim(),
        description: `Created from a page captured with the REA browser extension: ${capture.value.url}`,
        objective: '',
        analysis_types: ['website', ...(app.pageType === 'product' ? (['product'] as const) : [])],
        authorization_basis: project.basis,
        authorization_note: project.note.trim(),
        authorization_confirmed: true,
      },
    })
  } catch (e) {
    projectError.value = asUiError(e)
  } finally {
    creating.value = false
  }
}

// ---- AI summary ----
const confirm = ref<Extract<SummarizeResult, { kind: 'confirm' }> | null>(null)
const job = ref<Schemas['AiJob'] | null>(null)
const detail = ref<Schemas['AiJobDetailDoc'] | null>(null)
const aiError = ref<UiError | null>(null)
const starting = ref(false)
let timer: ReturnType<typeof setTimeout> | null = null
let polls = 0

async function summarize(confirmExternal: boolean): Promise<void> {
  if (!capture.value) return
  starting.value = true
  aiError.value = null
  detail.value = null
  try {
    const res = await call({ type: 'ai.summarize', capturedPageId: capture.value.id, confirmExternal })
    if (res.kind === 'confirm') {
      confirm.value = res
      return
    }
    confirm.value = null
    job.value = res.job
    polls = 0
    schedulePoll()
  } catch (e) {
    aiError.value = asUiError(e)
  } finally {
    starting.value = false
  }
}

function schedulePoll(): void {
  if (timer) clearTimeout(timer)
  timer = setTimeout(poll, polls < 10 ? 1500 : 4000)
}

async function poll(): Promise<void> {
  if (!job.value) return
  polls++
  try {
    detail.value = await call({ type: 'ai.job', jobId: job.value.id })
    const st = detail.value.job.status
    if ((st === 'queued' || st === 'running') && polls < 150) schedulePoll()
  } catch (e) {
    aiError.value = asUiError(e)
  }
}

onBeforeUnmount(() => { if (timer) clearTimeout(timer) })

interface SummaryOutput { summary?: string; key_points?: { text?: string; citations?: string[] }[]; unknowns?: string[] }
const output = computed<SummaryOutput | null>(() => {
  const o = detail.value?.job.output
  return o && typeof o === 'object' ? (o as SummaryOutput) : null
})
const running = computed(() => !!job.value && (!detail.value || ['queued', 'running'].includes(detail.value.job.status)))
</script>

<template>
  <SectionCard title="Analysis" subtitle="Analyze the captured page in REA. Start by saving a capture.">
    <Notice v-if="!connected" kind="warn">Pair the extension with REA to analyze pages.</Notice>
    <template v-else-if="!capture">
      <p class="muted">Analysis works on a saved capture of this page.</p>
      <button type="button" class="btn-primary w-full" :disabled="!app.extraction || app.capturing || !can('captures.write')" @click="saveCapture">
        <Spinner v-if="app.capturing" /> Save capture of this page
      </button>
      <p v-if="!app.extraction" class="muted">Read the page in the Capture section first.</p>
      <ErrorNote :error="app.captureError" />
    </template>
    <p v-else class="text-[13px]">Capture: <ExtLink :href="link(`/captures/${capture.id}`)">{{ capture.title || capture.url }}</ExtLink></p>
  </SectionCard>

  <SectionCard v-if="connected && capture && can('analysis.write')" title="Analysis project" subtitle="Creates a project with this capture as a source and queues REA's website analyzer.">
    <div class="space-y-2">
      <div>
        <label class="label" for="a-name">Project name</label>
        <input id="a-name" v-model="project.name" class="input" maxlength="200" />
      </div>
      <div>
        <label class="label" for="a-ws">Workspace</label>
        <select id="a-ws" v-model="project.workspace_id" class="input">
          <option v-for="w in workspaces" :key="w.id" :value="w.id">{{ w.name }}</option>
        </select>
      </div>
      <div>
        <label class="label" for="a-basis">Authorization basis</label>
        <select id="a-basis" v-model="project.basis" class="input">
          <option v-for="b in BASES" :key="b.value" :value="b.value">{{ b.label }}</option>
        </select>
      </div>
      <div v-if="noteRequired || project.note">
        <label class="label" for="a-note">Who authorized this analysis?</label>
        <textarea id="a-note" v-model="project.note" class="input" maxlength="2000" />
      </div>
      <label class="flex gap-2 text-[13px]">
        <input v-model="project.confirmed" type="checkbox" />
        I confirm I may analyze this material (it is ours, public, licensed or authorized).
      </label>
      <button
        type="button"
        class="btn-primary w-full"
        :disabled="creating || !project.confirmed || !project.workspace_id || !project.name.trim() || (noteRequired && project.note.trim().length < 10)"
        @click="createProject"
      >
        <Spinner v-if="creating" /> Create analysis project
      </button>
      <ErrorNote :error="projectError" />
      <Notice v-if="created" kind="success">
        Project created{{ created.source.run ? ' and analysis queued' : '' }}.
        <ExtLink :href="link(`/analysis/${created.project.id}`)">Open project</ExtLink>
      </Notice>
    </div>
  </SectionCard>

  <SectionCard v-if="connected && capture && aiEnabled && can('ai.use')" title="AI summary" subtitle="Runs REA's summarize tool on the captured page.">
    <Notice v-if="confirm" kind="warn">
      <p class="font-medium">This sends data to an external AI provider.</p>
      <dl class="grid grid-cols-[6rem_1fr] gap-x-2 text-xs">
        <dt>Provider</dt><dd>{{ confirm.provider.name }} ({{ confirm.provider.kind }})</dd>
        <dt>Host</dt><dd class="break-all">{{ confirm.provider.host }}</dd>
        <dt>Model</dt><dd>{{ confirm.provider.model }}</dd>
        <dt>Data sent</dt><dd>{{ confirm.dataCategories.join(', ') }}</dd>
      </dl>
      <p class="text-xs">{{ confirm.message }}</p>
      <div class="flex gap-2">
        <button type="button" class="btn-primary" :disabled="starting" data-testid="ai-confirm" @click="summarize(true)">Send and summarize</button>
        <button type="button" class="btn-secondary" @click="confirm = null">Cancel</button>
      </div>
    </Notice>
    <button v-else type="button" class="btn-primary w-full" :disabled="starting || running" @click="summarize(false)">
      <Spinner v-if="starting || running" /><Sparkles v-else class="size-4" aria-hidden="true" /> {{ running ? 'Summarizing…' : 'Summarize with AI' }}
    </button>
    <ErrorNote :error="aiError" />
    <div v-if="detail" class="space-y-2">
      <p class="muted">Status: {{ detail.job.status }}<template v-if="detail.job.provider_name"> · {{ detail.job.provider_name }} ({{ detail.job.model }})</template></p>
      <ErrorNote v-if="detail.job.status === 'failed'" :error="{ code: 'ai_failed', message: detail.job.error || 'The AI job failed.' }" />
      <div v-if="detail.job.status === 'succeeded'" class="space-y-2 rounded-md border border-violet-200 bg-violet-50 p-2">
        <p class="chip border-violet-300 bg-white text-violet-800">AI-generated</p>
        <p class="text-xs text-violet-900">{{ detail.ai_generated_notice }}</p>
        <p v-if="output?.summary" class="text-[13px] whitespace-pre-line">{{ output.summary }}</p>
        <p v-else-if="detail.job.output_text" class="text-[13px] whitespace-pre-line">{{ detail.job.output_text }}</p>
        <ul v-if="output?.key_points?.length" class="list-disc pl-4 text-[13px]">
          <li v-for="(k, i) in output.key_points" :key="i">{{ k.text }} <span v-if="k.citations?.length" class="muted">[{{ k.citations.join(', ') }}]</span></li>
        </ul>
        <div v-if="output?.unknowns?.length">
          <p class="text-xs font-medium">Not answered by the evidence</p>
          <ul class="list-disc pl-4 text-xs"><li v-for="(u, i) in output.unknowns" :key="i">{{ u }}</li></ul>
        </div>
      </div>
      <ExtLink :href="link(`/ai/jobs/${detail.job.id}`)">Open AI job in REA</ExtLink>
    </div>
  </SectionCard>
</template>
