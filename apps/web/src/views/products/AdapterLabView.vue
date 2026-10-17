<script setup lang="ts">
// Adapter lab: verify marketplace definitions and draft rule documents against saved product pages.
// The saved HTML is parsed into an inert document (DOMParser): it is never inserted into this page,
// its scripts never run and nothing it references is fetched.
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { FlaskConical, Play, Save, Upload } from 'lucide-vue-next'
import { parseRuleDocument, toAdapterConfig, type AdapterConfig, type RuleIssue } from '@rea/extractor'
import { api, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import StateViews from '@/components/ui/StateViews.vue'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import LabResults from '@/components/marketplaces/LabResults.vue'
import RulesAdapterFormModal from '@/components/marketplaces/RulesAdapterFormModal.vue'
import { MAX_LAB_HTML_BYTES, runLab, type LabMode, type LabRun } from '@/components/marketplaces/lab'
import { RULES_TEMPLATE } from '@/components/marketplaces/marketplaces'
import { useAuth } from '@/stores/auth'
import { formatBytes } from '@/lib/format'

const auth = useAuth()
const route = useRoute()
const qc = useQueryClient()
const canManage = computed(() => auth.can('settings.manage'))

const q = useQuery({ queryKey: ['adapters'], queryFn: ({ signal }) => api.get<Schemas['SourceAdapterItems']>('/adapters', undefined, signal) })
const converted = computed(() => (q.data.value?.items ?? []).map((a) => ({ a, r: toAdapterConfig(a) })))
const configs = computed<AdapterConfig[]>(() => converted.value.flatMap((x) => (x.r.ok ? [x.r.cfg] : [])))
const invalid = computed(() => converted.value.filter((x) => !x.r.ok).map((x) => x.a.name))
const choosable = computed(() => [...configs.value].sort((a, b) => a.name.localeCompare(b.name)))

const url = ref('')
const html = ref('')
const fileName = ref('')
const fileError = ref('')
const mode = ref<LabMode>('all')
const adapterKey = ref('')
const draft = ref(RULES_TEMPLATE)
const fallbacks = ref(true)
const run = ref<LabRun | null>(null)
const runError = ref('')
const runIssues = ref<RuleIssue[]>([])
const saveOpen = ref(false)

// "Test in lab" links pass ?adapter=<key>.
watch(
  [() => route.query.adapter, configs],
  ([key]) => {
    if (typeof key === 'string' && key && configs.value.some((c) => c.key === key) && !adapterKey.value) {
      adapterKey.value = key
      mode.value = 'one'
    }
  },
  { immediate: true },
)

const draftCheck = computed(() => parseRuleDocument(draft.value))

async function onFile(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  fileError.value = ''
  if (!file) return
  if (file.size > MAX_LAB_HTML_BYTES) {
    fileError.value = `${file.name} is ${formatBytes(file.size)}; saved pages are limited to ${formatBytes(MAX_LAB_HTML_BYTES)}.`
    input.value = ''
    return
  }
  html.value = await file.text()
  fileName.value = file.name
  if (!url.value) {
    // Browsers record the address in saved pages as <!-- saved from url=(NNNN)https://... -->.
    const m = /<!--\s*saved from url=\(\d+\)(https?:\/\/[^\s>]+)\s*-->/i.exec(html.value.slice(0, 2000))
    if (m) url.value = m[1]!
  }
}

function copyDefinition() {
  const cfg = configs.value.find((c) => c.key === adapterKey.value)
  if (cfg?.rules) {
    draft.value = JSON.stringify(cfg.rules, null, 2)
    mode.value = 'draft'
  }
}

function execute() {
  runError.value = ''
  runIssues.value = []
  const outcome = runLab({ html: html.value, url: url.value, mode: mode.value, adapterKey: adapterKey.value, draft: draft.value, fallbacks: fallbacks.value }, configs.value)
  if (outcome.ok) {
    run.value = outcome.run
  } else {
    run.value = null
    runError.value = outcome.error
    runIssues.value = outcome.issues ?? []
  }
}

const selected = computed(() => configs.value.find((c) => c.key === adapterKey.value) ?? null)
const draftHost = computed(() => {
  try {
    return new URL(url.value).hostname.replace(/^www\./, '')
  } catch {
    return ''
  }
})
async function onSaved() {
  await qc.invalidateQueries({ queryKey: ['adapters'] })
}
</script>

<template>
  <div>
    <PageHeader title="Adapter lab" subtitle="Check marketplace definitions and draft rule documents against product pages you saved." back="/marketplaces" />

    <p class="mb-4 max-w-3xl text-sm text-slate-600">
      Save a product page in your browser (<kbd>Ctrl</kbd>+<kbd>S</kbd>, “Webpage, HTML only” or “complete”), then paste or choose the file and enter the address it came from.
      The page is read here, in your browser, as inert data: its scripts never run and nothing it links to is loaded. Nothing is sent to REA.
    </p>

    <StateViews v-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <div v-else class="grid grid-cols-1 gap-6 xl:grid-cols-[minmax(0,26rem)_minmax(0,1fr)]">
      <form class="card space-y-4 self-start p-4" aria-label="Lab input" @submit.prevent="execute">
        <FormField v-slot="{ id }" label="Page address" required>
          <input :id="id" v-model="url" class="input font-mono text-xs" type="url" placeholder="https://www.example.com/item/123" required spellcheck="false" data-testid="lab-url" />
        </FormField>
        <FormField v-slot="{ id, describedby }" label="Saved page HTML" required :error="fileError" :hint="fileName ? `Loaded ${fileName} (${formatBytes(html.length)}).` : `Up to ${formatBytes(MAX_LAB_HTML_BYTES)}.`">
          <textarea :id="id" v-model="html" rows="8" class="input font-mono text-xs" spellcheck="false" :aria-describedby="describedby" placeholder="<!doctype html>…" data-testid="lab-html" />
        </FormField>
        <label class="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-slate-300 bg-white px-3 py-1.5 text-xs font-medium text-slate-700 shadow-xs hover:bg-slate-50">
          <Upload class="h-4 w-4" aria-hidden="true" /> Choose a saved .html file
          <input type="file" accept=".html,.htm,text/html" class="sr-only" data-testid="lab-file" @change="onFile" />
        </label>

        <fieldset class="space-y-2">
          <legend class="label">Adapters</legend>
          <label class="flex items-start gap-2 text-sm"><input v-model="mode" type="radio" value="all" class="mt-1" /> <span>All enabled adapters, as the extension runs them on this address</span></label>
          <label class="flex items-start gap-2 text-sm"><input v-model="mode" type="radio" value="one" class="mt-1" data-testid="lab-mode-one" /> <span>One adapter or marketplace definition</span></label>
          <div v-if="mode === 'one'" class="space-y-2 pl-6">
            <label class="sr-only" for="lab-adapter">Adapter to test</label>
            <select id="lab-adapter" v-model="adapterKey" class="input" data-testid="lab-adapter">
              <option value="" disabled>Choose an adapter</option>
              <option v-for="c in choosable" :key="c.key" :value="c.key">{{ c.name }} ({{ c.kind }}{{ c.enabled ? '' : ', disabled' }})</option>
            </select>
            <UiButton v-if="selected?.rules" size="sm" @click="copyDefinition">Copy its rule document into a draft</UiButton>
          </div>
          <label class="flex items-start gap-2 text-sm"><input v-model="mode" type="radio" value="draft" class="mt-1" data-testid="lab-mode-draft" /> <span>A draft rule document</span></label>
          <div v-if="mode === 'draft'" class="space-y-2 pl-6">
            <label class="sr-only" for="lab-draft">Draft rule document</label>
            <textarea id="lab-draft" v-model="draft" rows="14" class="input font-mono text-xs" spellcheck="false" data-testid="lab-draft" />
            <p v-if="draftCheck.ok" class="text-xs text-green-700" data-testid="lab-draft-valid">Valid rule document.</p>
            <ul v-else class="list-disc pl-5 text-xs text-red-700" role="alert" data-testid="lab-draft-issues">
              <li v-for="(i, n) in draftCheck.issues.slice(0, 8)" :key="n"><code v-if="i.path">{{ i.path }}</code> {{ i.message }}</li>
            </ul>
          </div>
          <label v-if="mode !== 'all'" class="flex items-center gap-2 pl-6 text-xs text-slate-600">
            <input v-model="fallbacks" type="checkbox" class="rounded border-slate-300" /> Also run JSON-LD, microdata, Open Graph and generic fallbacks
          </label>
        </fieldset>
        <p v-if="invalid.length" class="text-xs text-red-700">Not runnable because their rule documents are invalid: {{ invalid.join(', ') }}.</p>

        <div class="flex flex-wrap gap-2">
          <UiButton type="submit" variant="primary" data-testid="lab-run"><Play class="h-4 w-4" aria-hidden="true" /> Read the page</UiButton>
          <UiButton v-if="canManage && mode === 'draft' && draftCheck.ok" @click="saveOpen = true"><Save class="h-4 w-4" aria-hidden="true" /> Save draft as a rules adapter</UiButton>
        </div>
        <div v-if="runError" class="rounded-md bg-red-50 px-3 py-2 text-sm text-red-800" role="alert" data-testid="lab-error">
          {{ runError }}
          <ul v-if="runIssues.length" class="mt-1 list-disc pl-5 text-xs">
            <li v-for="(i, n) in runIssues.slice(0, 8)" :key="n"><code v-if="i.path">{{ i.path }}</code> {{ i.message }}</li>
          </ul>
        </div>
      </form>

      <div>
        <LabResults v-if="run" :run="run" />
        <div v-else class="card"><StateViews kind="empty" title="Nothing read yet" message="Enter a saved page and choose “Read the page” to see every field, its confidence and the rule or selector it came from."><FlaskConical class="h-5 w-5 text-slate-400" aria-hidden="true" /></StateViews></div>
      </div>
    </div>

    <RulesAdapterFormModal :open="saveOpen" :adapter="null" :draft="{ rules: draft, domains: draftHost ? [draftHost, `*.${draftHost}`] : [] }" @close="saveOpen = false" @saved="onSaved" />
  </div>
</template>
