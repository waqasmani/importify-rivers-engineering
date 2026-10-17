<script setup lang="ts">
// API documentation viewer built from the OpenAPI and API-example analyzers: servers, security
// schemes, an endpoint table with parameters, auth and request/response shapes, and schemas.
import { computed, ref } from 'vue'
import { ChevronRight, Lock, LockOpen } from 'lucide-vue-next'
import UiBadge from '@/components/ui/UiBadge.vue'
import StateViews from '@/components/ui/StateViews.vue'
import FindingLink from '../FindingLink.vue'
import { arr, byCategory, dataOf, num, obj, pretty, str, strings, type FindingRow } from '../findingData'

const props = defineProps<{ kind: string; findings: FindingRow[] }>()
const examples = computed(() => props.kind === 'api_examples')

interface Endpoint {
  id: string
  classification: string
  method: string
  path: string
  host: string
  summary: string
  operationId: string
  tags: string[]
  parameters: string[]
  requestBody: string
  responses: string[]
  secured: boolean | null
  statuses: number[]
  requestShape: unknown
  responseShape: unknown
}

const endpoints = computed<Endpoint[]>(() =>
  byCategory(props.findings, 'endpoint')
    .filter((f) => str(dataOf(f).method))
    .map((f) => {
      const d = dataOf(f)
      return {
        id: f.id, classification: f.classification, method: str(d.method).toUpperCase(), path: str(d.path), host: str(d.host),
        summary: str(d.summary) || (examples.value ? '' : f.detail), operationId: str(d.operation_id), tags: strings(d.tags), parameters: strings(d.parameters),
        requestBody: str(d.request_body), responses: strings(d.responses), secured: typeof d.secured === 'boolean' ? d.secured : null,
        statuses: arr(d.statuses).map(num), requestShape: d.request_shape ?? null, responseShape: d.response_shape ?? null,
      }
    })
    .sort((a, b) => a.path.localeCompare(b.path) || a.method.localeCompare(b.method)),
)
const notes = computed(() => byCategory(props.findings, 'endpoint').filter((f) => !str(dataOf(f).method)))
const apiInfo = computed(() => byCategory(props.findings, 'api').map((f) => ({ f, d: dataOf(f) })))
const servers = computed(() => apiInfo.value.flatMap((x) => strings(x.d.servers)))
const title = computed(() => apiInfo.value.find((x) => str(x.d.title) || str(x.d.spec_version)))
const schemes = computed(() => byCategory(props.findings, 'auth'))
const schemas = computed(() =>
  byCategory(props.findings, 'schema').map((f) => ({ f, name: str(dataOf(f).name) || f.title, fields: arr(dataOf(f).fields).map((x) => obj(x)) })),
)

const q = ref('')
const method = ref('')
const tag = ref('')
const methods = computed(() => [...new Set(endpoints.value.map((e) => e.method))].sort())
const allTags = computed(() => [...new Set(endpoints.value.flatMap((e) => e.tags))].sort())
const filtered = computed(() => {
  const t = q.value.trim().toLowerCase()
  return endpoints.value.filter(
    (e) =>
      (!method.value || e.method === method.value) &&
      (!tag.value || e.tags.includes(tag.value)) &&
      (!t || `${e.method} ${e.path} ${e.summary} ${e.operationId} ${e.parameters.join(' ')}`.toLowerCase().includes(t)),
  )
})
const open = ref<Record<string, boolean>>({})
const methodTone: Record<string, 'green' | 'blue' | 'amber' | 'red' | 'violet' | 'slate'> = { GET: 'blue', POST: 'green', PUT: 'amber', PATCH: 'amber', DELETE: 'red' }
</script>

<template>
  <div class="space-y-5" data-testid="api-viewer">
    <div v-if="title || servers.length || schemes.length" class="grid grid-cols-1 gap-3 md:grid-cols-3">
      <div v-if="title" class="rounded-md border border-slate-200 p-3 text-sm">
        <p class="text-xs text-slate-500">API</p>
        <p class="font-semibold text-slate-900">{{ str(title.d.title) || 'Untitled' }} <span class="font-normal text-slate-500">{{ str(title.d.version) }}</span></p>
        <p v-if="str(title.d.spec_version)" class="text-xs text-slate-500">OpenAPI/Swagger {{ str(title.d.spec_version) }}</p>
        <p v-if="title.f.detail" class="mt-1 line-clamp-4 text-xs text-slate-600">{{ title.f.detail }}</p>
      </div>
      <div v-if="servers.length" class="rounded-md border border-slate-200 p-3 text-sm">
        <p class="text-xs text-slate-500">Servers</p>
        <ul class="mt-1 space-y-0.5"><li v-for="s in servers" :key="s" class="font-mono text-xs break-all">{{ s }}</li></ul>
      </div>
      <div v-if="schemes.length" class="rounded-md border border-slate-200 p-3 text-sm">
        <p class="text-xs text-slate-500">Authentication</p>
        <ul class="mt-1 space-y-1">
          <li v-for="s in schemes" :key="s.id" class="flex flex-wrap items-center gap-1">
            <span class="font-medium">{{ s.title.replace(/^Security scheme: /, '') }}</span>
            <span v-if="s.evidence" class="font-mono text-xs text-slate-600">{{ s.evidence }}</span>
            <FindingLink :id="s.id" :classification="s.classification" :label="s.title" />
          </li>
        </ul>
      </div>
    </div>

    <section aria-labelledby="api-endpoints">
      <div class="mb-2 flex flex-col gap-2 sm:flex-row sm:items-center">
        <h3 id="api-endpoints" class="text-sm font-semibold text-slate-800">Endpoints ({{ filtered.length }}<template v-if="filtered.length !== endpoints.length"> of {{ endpoints.length }}</template>)</h3>
        <div class="flex flex-1 flex-wrap gap-2 sm:justify-end">
          <label for="api-q" class="sr-only">Filter endpoints</label>
          <input id="api-q" v-model="q" type="search" class="input max-w-xs" placeholder="Filter by path, summary or parameter" />
          <label for="api-method" class="sr-only">Method</label>
          <select id="api-method" v-model="method" class="input w-auto">
            <option value="">All methods</option>
            <option v-for="m in methods" :key="m" :value="m">{{ m }}</option>
          </select>
          <template v-if="allTags.length">
            <label for="api-tag" class="sr-only">Tag</label>
            <select id="api-tag" v-model="tag" class="input w-auto">
              <option value="">All tags</option>
              <option v-for="t in allTags" :key="t" :value="t">{{ t }}</option>
            </select>
          </template>
        </div>
      </div>
      <StateViews v-if="endpoints.length === 0" kind="empty" title="No endpoints found" message="The analyzer did not find operations in this source yet. Check the Runs tab of the project if the analysis is still running or failed." />
      <div v-else class="overflow-x-auto rounded-md border border-slate-200">
        <table class="min-w-full divide-y divide-slate-200 text-sm">
          <thead class="bg-slate-50">
            <tr>
              <th scope="col" class="table-th w-8"><span class="sr-only">Details</span></th>
              <th scope="col" class="table-th">Endpoint</th>
              <th scope="col" class="table-th">Summary</th>
              <th scope="col" class="table-th">Parameters</th>
              <th scope="col" class="table-th">Auth</th>
              <th scope="col" class="table-th">{{ examples ? 'Observed statuses' : 'Request / responses' }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100 bg-white">
            <template v-for="e in filtered" :key="e.id">
              <tr class="align-top" data-testid="api-endpoint">
                <td class="px-2 py-2">
                  <button type="button" class="rounded p-1 text-slate-500 hover:bg-slate-100" :aria-expanded="!!open[e.id]" :aria-label="`Details for ${e.method} ${e.path}`" @click="open[e.id] = !open[e.id]">
                    <ChevronRight class="h-4 w-4 transition-transform" :class="open[e.id] ? 'rotate-90' : ''" />
                  </button>
                </td>
                <td class="table-td whitespace-nowrap"><UiBadge :tone="methodTone[e.method] ?? 'slate'" class="font-mono">{{ e.method }}</UiBadge> <code class="text-xs">{{ e.path }}</code></td>
                <td class="table-td min-w-48">{{ e.summary || '—' }}</td>
                <td class="table-td min-w-40 text-xs">
                  <ul v-if="e.parameters.length"><li v-for="p in e.parameters" :key="p" class="font-mono">{{ p }}</li></ul>
                  <span v-else class="text-slate-400">None</span>
                </td>
                <td class="table-td whitespace-nowrap text-xs">
                  <span v-if="e.secured === true" class="inline-flex items-center gap-1 text-amber-800"><Lock class="h-3.5 w-3.5" aria-hidden="true" /> Required</span>
                  <span v-else-if="e.secured === false" class="inline-flex items-center gap-1 text-slate-600"><LockOpen class="h-3.5 w-3.5" aria-hidden="true" /> None declared</span>
                  <span v-else class="text-slate-500" title="Credentials are removed from recorded traffic, so authentication cannot be observed.">Unknown</span>
                </td>
                <td class="table-td min-w-48 text-xs">
                  <template v-if="examples">{{ e.statuses.join(', ') || '—' }}</template>
                  <template v-else>
                    <p v-if="e.requestBody"><span class="text-slate-500">Body:</span> <code>{{ e.requestBody }}</code></p>
                    <p v-for="r in e.responses" :key="r"><code>{{ r }}</code></p>
                  </template>
                </td>
              </tr>
              <tr v-if="open[e.id]" class="bg-slate-50/60">
                <td />
                <td colspan="5" class="px-3 py-3 text-xs">
                  <div class="flex flex-wrap items-center gap-2">
                    <FindingLink :id="e.id" :classification="e.classification" :label="`${e.method} ${e.path}`" />
                    <span v-if="e.operationId">Operation <code>{{ e.operationId }}</code></span>
                    <span v-if="e.host">Host <code>{{ e.host }}</code></span>
                    <UiBadge v-for="t in e.tags" :key="t" tone="slate">{{ t }}</UiBadge>
                  </div>
                  <div v-if="examples" class="mt-2 grid grid-cols-1 gap-2 md:grid-cols-2">
                    <div v-if="e.requestShape"><p class="font-medium text-slate-600">Request shape</p><pre class="mt-1 max-h-64 overflow-auto rounded bg-white p-2 font-mono">{{ pretty(e.requestShape) }}</pre></div>
                    <div v-if="e.responseShape"><p class="font-medium text-slate-600">Response shape</p><pre class="mt-1 max-h-64 overflow-auto rounded bg-white p-2 font-mono">{{ pretty(e.responseShape) }}</pre></div>
                    <p v-if="!e.requestShape && !e.responseShape" class="text-slate-500">No JSON bodies were recorded for this endpoint.</p>
                  </div>
                  <p v-if="examples" class="mt-2 text-slate-500">Only the structure of bodies is kept; values, headers and cookies are not stored.</p>
                </td>
              </tr>
            </template>
          </tbody>
        </table>
      </div>
      <ul v-if="notes.length" class="mt-2 space-y-1 text-xs text-slate-600">
        <li v-for="n in notes" :key="n.id" class="flex flex-wrap items-center gap-1"><FindingLink :id="n.id" :classification="n.classification" :label="n.title" /> {{ n.title }}<span v-if="n.detail" class="text-slate-500">— {{ n.detail }}</span></li>
      </ul>
    </section>

    <section v-if="schemas.length" aria-labelledby="api-schemas">
      <h3 id="api-schemas" class="mb-2 text-sm font-semibold text-slate-800">Schemas ({{ schemas.length }})</h3>
      <div class="grid grid-cols-1 gap-2 md:grid-cols-2">
        <details v-for="s in schemas" :key="s.f.id" class="rounded-md border border-slate-200 bg-white p-2 text-sm">
          <summary class="cursor-pointer font-medium text-slate-800">{{ s.name }} <span class="text-xs font-normal text-slate-500">· {{ s.fields.length }} fields</span></summary>
          <p v-if="s.f.detail" class="mt-1 text-xs text-slate-600">{{ s.f.detail }}</p>
          <table class="mt-2 min-w-full text-xs">
            <tbody class="divide-y divide-slate-100">
              <tr v-for="fl in s.fields" :key="str(fl.name)">
                <td class="py-1 pr-2 font-mono">{{ str(fl.name) }}<span v-if="fl.required === true" class="text-red-600" title="required"> *</span></td>
                <td class="py-1 text-slate-600">{{ str(fl.type) || '—' }}</td>
              </tr>
            </tbody>
          </table>
          <FindingLink :id="s.f.id" :classification="s.f.classification" :label="s.name" class="mt-2" />
        </details>
      </div>
    </section>
  </div>
</template>
