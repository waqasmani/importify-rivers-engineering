<script setup lang="ts">
// Collects one analysis source of any supported kind. The parent decides when to send it (the
// wizard queues drafts; the project page stores them right away) and passes server errors back.
import { computed, useId } from 'vue'
import { Code, FileArchive, FileText, Globe, Image, KeyRound, ListOrdered, Network, ScanSearch, Server, FileCode, Braces } from 'lucide-vue-next'
import FormField from '@/components/ui/FormField.vue'
import { formatBytes } from '@/lib/format'
import CapturePicker from './CapturePicker.vue'
import WorkflowStepsEditor from './WorkflowStepsEditor.vue'
import VisibleOnlyNotice from './VisibleOnlyNotice.vue'
import { CODE_NOTICE, CREDENTIALS_NOTICE, SOURCE_KINDS, sourceKindMeta } from './options'
import type { SourceDraft } from './sources'

const draft = defineModel<SourceDraft>({ required: true })
const props = withDefaults(defineProps<{ kinds: string[]; errors?: Record<string, string>; maxUploadMb?: number }>(), { errors: () => ({}), maxUploadMb: 0 })
const emit = defineEmits<{ 'clear-error': [field: string] }>()
const uid = useId()

const icons: Record<string, unknown> = {
  captured_page: ScanSearch, url: Globe, html: Code, text: FileText, document: FileText, workflow: ListOrdered,
  openapi: Braces, api_examples: Network, screenshot: Image, repository_archive: FileArchive, repository_path: Server,
}
const options = computed(() => SOURCE_KINDS.filter((k) => props.kinds.includes(k.kind)))
const meta = computed(() => sourceKindMeta(draft.value.kind))
const showUpload = computed(() => meta.value?.input === 'upload' || (meta.value?.input === 'paste_or_upload' && draft.value.mode === 'upload'))
const showPaste = computed(() => meta.value?.input === 'paste' || (meta.value?.input === 'paste_or_upload' && draft.value.mode === 'paste'))

const pastePlaceholder: Record<string, string> = {
  html: '<!doctype html>…',
  text: 'Paste notes, requirements or other text…',
  openapi: 'openapi: 3.0.0\ninfo:\n  title: …',
  api_examples: '[{"method": "GET", "url": "https://api.example.com/items/1", "status": 200, "response_body": {…}}]',
}
const uploadHint = computed(() => {
  const limit = props.maxUploadMb ? `Up to ${props.maxUploadMb} MB` : 'Size limits apply'
  switch (draft.value.kind) {
    case 'repository_archive':
      return `${limit}. .zip or .tar.gz. ${CODE_NOTICE}`
    case 'document':
      return `${limit}. PDF, DOCX, Markdown, HTML or text. Scanned PDFs without text cannot be read (no OCR).`
    case 'screenshot':
      return `${limit}. PNG, JPEG, GIF or WebP.`
    case 'api_examples':
      return 'Up to 5 MB. HAR or a JSON array of {method, url, status, request_body, response_body}. Headers, cookies and credential-like fields are removed before storage; the original file is not kept.'
    case 'openapi':
      return 'Up to 5 MB. OpenAPI 3 or Swagger 2 in JSON or YAML.'
    case 'html':
      return 'Up to 5 MB. A saved .html page. It is parsed as text and never rendered or executed.'
  }
  return limit
})

function pick(kind: string) {
  if (draft.value.kind === kind) return
  draft.value.kind = kind
  draft.value.mode = 'paste'
  draft.value.file = null
  emit('clear-error', 'kind')
}
function onFile(e: Event) {
  const f = (e.target as HTMLInputElement).files?.[0] ?? null
  draft.value.file = f
  emit('clear-error', 'file')
}
</script>

<template>
  <div class="space-y-4">
    <fieldset>
      <legend class="label">Type of source</legend>
      <div class="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3" role="radiogroup">
        <label
          v-for="k in options"
          :key="k.kind"
          class="flex cursor-pointer items-start gap-2 rounded-md border p-2.5 text-sm focus-within:ring-2 focus-within:ring-brand-500"
          :class="draft.kind === k.kind ? 'border-brand-500 bg-brand-50' : 'border-slate-200 bg-white hover:border-slate-300'"
          :data-testid="`kind-${k.kind}`"
        >
          <input type="radio" :name="`${uid}-kind`" :value="k.kind" :checked="draft.kind === k.kind" class="mt-1" @change="pick(k.kind)" />
          <component :is="icons[k.kind] ?? FileCode" class="mt-0.5 h-4 w-4 shrink-0 text-slate-500" aria-hidden="true" />
          <span>
            <span class="block font-medium text-slate-900">{{ k.label }}</span>
            <span class="block text-xs text-slate-500">{{ k.description }}</span>
          </span>
        </label>
      </div>
      <p v-if="errors.kind" class="mt-1 text-xs text-red-600" role="alert">{{ errors.kind }}</p>
    </fieldset>

    <template v-if="meta">
      <VisibleOnlyNotice v-if="meta.visibleOnly" compact />
      <p v-if="meta.code" class="flex items-start gap-2 rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-xs text-emerald-900" role="note">
        <Code class="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" /> {{ CODE_NOTICE }}
      </p>
      <p class="flex items-start gap-2 text-xs text-slate-500"><KeyRound class="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden="true" /> {{ CREDENTIALS_NOTICE }}</p>

      <FormField v-slot="{ id, describedby }" label="Title" hint="Optional. Defaults to the file name, URL or page title." :error="errors.title">
        <input :id="id" v-model="draft.title" class="input" maxlength="300" :aria-describedby="describedby" />
      </FormField>

      <FormField v-if="meta.input === 'url'" v-slot="{ id, describedby, invalid }" label="Page URL" required :error="errors.url" hint="Only public pages. The server does not sign in, solve challenges or bypass access restrictions.">
        <input :id="id" v-model="draft.url" type="url" inputmode="url" class="input" :class="{ 'input-invalid': invalid }" placeholder="https://example.com/page" :aria-describedby="describedby" :aria-invalid="invalid" @input="emit('clear-error', 'url')" />
      </FormField>

      <div v-if="meta.input === 'capture'">
        <p class="label" :id="`${uid}-cap`">Captured page<span class="text-red-600" aria-hidden="true"> *</span></p>
        <CapturePicker v-model="draft.capturedPageId" :label="draft.capturedLabel" :error="errors.captured_page_id" @select="(l) => { draft.capturedLabel = l; emit('clear-error', 'captured_page_id') }" />
      </div>

      <fieldset v-if="meta.input === 'paste_or_upload'" class="flex flex-wrap gap-4 text-sm">
        <legend class="sr-only">How to provide the content</legend>
        <label class="flex items-center gap-2"><input v-model="draft.mode" type="radio" value="paste" /> Paste text</label>
        <label class="flex items-center gap-2"><input v-model="draft.mode" type="radio" value="upload" /> Upload a file</label>
      </fieldset>

      <FormField v-if="showPaste" v-slot="{ id, describedby, invalid }" :label="draft.kind === 'html' ? 'Page source' : draft.kind === 'openapi' ? 'API description (JSON or YAML)' : draft.kind === 'api_examples' ? 'Examples (JSON or HAR)' : 'Text'" required :error="errors.content" :hint="draft.kind === 'api_examples' ? 'Headers, cookies and credential-like fields are removed before storage.' : 'Up to 5 MB.'">
        <textarea :id="id" v-model="draft.content" rows="10" class="input font-mono text-xs" :class="{ 'input-invalid': invalid }" spellcheck="false" :placeholder="pastePlaceholder[draft.kind]" :aria-describedby="describedby" :aria-invalid="invalid" @input="emit('clear-error', 'content')" />
      </FormField>

      <FormField v-if="showUpload" v-slot="{ id, describedby, invalid }" label="File" required :error="errors.file" :hint="uploadHint">
        <input :id="id" :key="`${draft.key}-${draft.kind}`" type="file" :accept="meta.accept" class="block w-full text-sm text-slate-700 file:mr-3 file:rounded-md file:border file:border-slate-300 file:bg-white file:px-3 file:py-1.5 file:text-sm file:font-medium hover:file:bg-slate-50" :aria-describedby="describedby" :aria-invalid="invalid" @change="onFile" />
        <p v-if="draft.file" class="mt-1 text-xs text-slate-600" data-testid="file-chosen">{{ draft.file.name }} · {{ formatBytes(draft.file.size) }}</p>
      </FormField>

      <div v-if="meta.input === 'steps'">
        <p class="label">Steps, in order<span class="text-red-600" aria-hidden="true"> *</span></p>
        <p class="mb-2 text-xs text-slate-500">Record what you observed. Each step needs an action; describe data by name, never by value.</p>
        <WorkflowStepsEditor v-model="draft.steps" :error="errors.steps" />
      </div>

      <FormField v-if="meta.input === 'path'" v-slot="{ id, describedby, invalid }" label="Directory on the server" required :error="errors.path" hint="Must be inside a directory the operator allowed with REA_REPO_ROOTS.">
        <input :id="id" v-model="draft.path" class="input font-mono" :class="{ 'input-invalid': invalid }" placeholder="/srv/repos/my-app" :aria-describedby="describedby" :aria-invalid="invalid" @input="emit('clear-error', 'path')" />
      </FormField>
    </template>
  </div>
</template>
