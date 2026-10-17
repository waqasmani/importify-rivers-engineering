<script setup lang="ts">
// Visible website analysis: the page and its metadata, navigation, forms (field names only),
// technologies seen in the markup, third-party hosts and structured data. Everything here
// describes what a visitor's browser receives, not how the site is built.
import { computed } from 'vue'
import { ExternalLink } from 'lucide-vue-next'
import StateViews from '@/components/ui/StateViews.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import FindingLink from '../FindingLink.vue'
import ConfidenceMeter from '../ConfidenceMeter.vue'
import VisibleOnlyNotice from '../VisibleOnlyNotice.vue'
import { arr, byCategory, dataOf, obj, str, strings, type FindingRow } from '../findingData'
import { formatDateTime } from '@/lib/format'

const props = defineProps<{ url?: string | null; metadata?: Record<string, unknown>; capturedPageId?: string | null; findings: FindingRow[]; canSeeCaptures?: boolean }>()

const meta = computed(() => {
  const f = byCategory(props.findings, 'metadata').find((x) => x.title === 'Page metadata')
  return f ? { f, d: dataOf(f) } : null
})
const metaRows = computed(() => {
  const d = meta.value?.d ?? {}
  return Object.entries(d).filter(([, v]) => str(v)).map(([k, v]) => [k, str(v)] as [string, string])
})
const nav = computed(() => byCategory(props.findings, 'navigation').flatMap((f) => arr(dataOf(f).links).map((l) => ({ text: str(obj(l).text), href: str(obj(l).href) }))))
const forms = computed(() =>
  byCategory(props.findings, 'form').map((f) => ({
    f, method: str(dataOf(f).method), action: str(dataOf(f).action), fields: arr(dataOf(f).fields).map((x) => ({ name: str(obj(x).name), type: str(obj(x).type), required: obj(x).required === true })),
  })),
)
const tech = computed(() => byCategory(props.findings, 'technology'))
const hosts = computed(() => byCategory(props.findings, 'integration').flatMap((f) => strings(dataOf(f).hosts).map((h) => ({ h, f }))))
const structured = computed(() => byCategory(props.findings, 'structured_data', 'product', 'organization'))
const others = computed(() => byCategory(props.findings, 'authentication', 'accessibility', 'selection', 'metadata').filter((f) => f.title !== 'Page metadata'))
const finalUrl = computed(() => str(props.metadata?.final_url))
const fetchedAt = computed(() => str(props.metadata?.fetched_at))
const isHttp = (u: string) => /^https?:\/\//i.test(u)
</script>

<template>
  <div class="space-y-5" data-testid="website-viewer">
    <VisibleOnlyNotice />
    <section aria-labelledby="web-page" class="rounded-md border border-slate-200 p-3 text-sm">
      <h3 id="web-page" class="mb-2 text-sm font-semibold text-slate-800">Page</h3>
      <dl class="grid grid-cols-1 gap-x-4 gap-y-1 sm:grid-cols-[9rem_1fr]">
        <template v-if="url">
          <dt class="text-slate-500">URL</dt>
          <dd class="break-all"><a v-if="isHttp(url)" :href="url" target="_blank" rel="noopener noreferrer nofollow" class="inline-flex items-center gap-1 text-brand-700 hover:underline">{{ url }} <ExternalLink class="h-3 w-3" aria-hidden="true" /></a><span v-else>{{ url }}</span></dd>
        </template>
        <template v-if="finalUrl && finalUrl !== url"><dt class="text-slate-500">Final URL</dt><dd class="break-all">{{ finalUrl }}</dd></template>
        <template v-if="fetchedAt"><dt class="text-slate-500">Fetched</dt><dd>{{ formatDateTime(fetchedAt) }} by the REA server, without credentials</dd></template>
        <template v-if="capturedPageId">
          <dt class="text-slate-500">Captured page</dt>
          <dd><RouterLink v-if="canSeeCaptures" :to="`/captures/${capturedPageId}`" class="text-brand-700 hover:underline">Open the capture</RouterLink><span v-else>Captured with the browser extension</span></dd>
        </template>
        <template v-for="[k, v] in metaRows" :key="k"><dt class="text-slate-500">{{ k }}</dt><dd class="break-words">{{ v }}</dd></template>
      </dl>
      <FindingLink v-if="meta" :id="meta.f.id" :classification="meta.f.classification" label="Page metadata" class="mt-2" />
    </section>

    <StateViews v-if="findings.length === 0" kind="empty" title="No page details yet" message="Details appear when the website analysis finishes." />
    <template v-else>
      <div class="grid grid-cols-1 gap-5 lg:grid-cols-2">
        <section aria-labelledby="web-tech">
          <h3 id="web-tech" class="mb-2 text-sm font-semibold text-slate-800">Technologies seen in the page</h3>
          <ul v-if="tech.length" class="space-y-1.5 text-sm">
            <li v-for="t in tech" :key="t.id" class="flex flex-wrap items-center gap-2">
              <span class="font-medium">{{ str(dataOf(t).technology) || t.title }}</span>
              <span v-if="str(dataOf(t).kind)" class="text-xs text-slate-500">{{ str(dataOf(t).kind).replace(/_/g, ' ') }}</span>
              <FindingLink :id="t.id" :classification="t.classification" :label="t.title" />
              <ConfidenceMeter :value="t.confidence" />
            </li>
          </ul>
          <p v-else class="text-sm text-slate-500">No technology markers were found.</p>
          <p class="mt-2 text-xs text-slate-500">Markers show what the browser receives; they can come from embeds and do not prove how the server is built.</p>
          <div v-if="hosts.length" class="mt-4">
            <h4 class="mb-1 text-xs font-medium text-slate-500">Third-party script and style hosts</h4>
            <ul class="flex flex-wrap gap-1"><li v-for="x in hosts" :key="x.h"><UiBadge tone="slate" class="font-mono">{{ x.h }}</UiBadge></li></ul>
          </div>
        </section>
        <section aria-labelledby="web-nav">
          <h3 id="web-nav" class="mb-2 text-sm font-semibold text-slate-800">Navigation ({{ nav.length }})</h3>
          <ul v-if="nav.length" class="max-h-64 space-y-0.5 overflow-auto text-sm">
            <li v-for="(l, i) in nav" :key="i" class="flex gap-2"><span class="font-medium">{{ l.text }}</span><span class="truncate font-mono text-xs text-slate-500">{{ l.href }}</span></li>
          </ul>
          <p v-else class="text-sm text-slate-500">No navigation links were found.</p>
        </section>
      </div>

      <section v-if="forms.length" aria-labelledby="web-forms">
        <h3 id="web-forms" class="mb-2 text-sm font-semibold text-slate-800">Forms ({{ forms.length }})</h3>
        <p class="mb-2 text-xs text-slate-500">Field names and types only. Values are never captured, and REA never submits forms.</p>
        <ul class="grid grid-cols-1 gap-2 md:grid-cols-2">
          <li v-for="fm in forms" :key="fm.f.id" class="rounded-md border border-slate-200 p-2 text-sm">
            <div class="flex flex-wrap items-center gap-2"><UiBadge tone="blue">{{ fm.method || 'GET' }}</UiBadge><code class="text-xs break-all">{{ fm.action || '(same page)' }}</code><FindingLink :id="fm.f.id" :classification="fm.f.classification" :label="fm.f.title" /></div>
            <ul class="mt-1 text-xs"><li v-for="fl in fm.fields" :key="fl.name + fl.type" class="font-mono">{{ fl.name || '(unnamed)' }}: {{ fl.type }}<span v-if="fl.required" class="text-red-600"> *</span></li></ul>
          </li>
        </ul>
      </section>

      <section v-if="structured.length" aria-labelledby="web-sd">
        <h3 id="web-sd" class="mb-2 text-sm font-semibold text-slate-800">Structured data ({{ structured.length }})</h3>
        <ul class="space-y-2">
          <li v-for="s in structured" :key="s.id" class="rounded-md border border-slate-200 p-2 text-sm">
            <div class="flex flex-wrap items-center gap-2"><span class="font-medium">{{ s.title }}</span><FindingLink :id="s.id" :classification="s.classification" :label="s.title" /></div>
            <details v-if="s.evidence" class="mt-1">
              <summary class="cursor-pointer text-xs text-slate-600">Show declared data</summary>
              <pre class="mt-1 max-h-72 overflow-auto rounded bg-slate-50 p-2 font-mono text-[11px] whitespace-pre-wrap">{{ s.evidence }}</pre>
            </details>
          </li>
        </ul>
      </section>

      <section v-if="others.length" aria-labelledby="web-other">
        <h3 id="web-other" class="mb-2 text-sm font-semibold text-slate-800">Other observations</h3>
        <ul class="space-y-1 text-sm">
          <li v-for="o in others" :key="o.id" class="flex flex-wrap items-center gap-2"><FindingLink :id="o.id" :classification="o.classification" :label="o.title" /> {{ o.title }}</li>
        </ul>
      </section>
    </template>
  </div>
</template>
