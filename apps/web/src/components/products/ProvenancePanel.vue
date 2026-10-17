<script setup lang="ts">
// Where the product data came from: source page, extraction method and confidence, evidence,
// timestamps, the captured page and the import history.
import { computed } from 'vue'
import { ExternalLink } from 'lucide-vue-next'
import type { Schemas } from '@/api/client'
import UiBadge from '@/components/ui/UiBadge.vue'
import { formatDateTime, percent, relativeTime, titleCase } from '@/lib/format'
import { asRecord, numberMap, safeHref, stringMap, DECISION_LABELS, IMPORT_STATUS_LABELS } from './product-utils'

const props = defineProps<{ product: Schemas['ProductDetail'] }>()

const p = computed(() => props.product)
const sourceHref = computed(() => safeHref(p.value.source_url))
const canonicalHref = computed(() => safeHref(p.value.canonical_url))
const evidence = computed(() => Object.entries(stringMap(p.value.source_evidence)))
const imports = computed(() => [...(p.value.imports ?? [])].sort((a, b) => b.created_at.localeCompare(a.created_at)))
// Per-field confidence comes from the most recent import whose payload reported it.
const fieldConfidence = computed(() => {
  for (const imp of imports.value) {
    const fc = numberMap(asRecord(imp.payload).field_confidence)
    if (Object.keys(fc).length) return Object.entries(fc).sort((a, b) => a[0].localeCompare(b[0]))
  }
  return []
})
const confidence = computed(() => (p.value.extraction_confidence === null ? null : Number(p.value.extraction_confidence)))
function tone(v: number): string {
  if (v >= 0.8) return 'bg-green-500'
  if (v >= 0.5) return 'bg-amber-500'
  return 'bg-red-500'
}
</script>

<template>
  <section class="card p-4" aria-labelledby="provenance-heading" data-testid="provenance">
    <h2 id="provenance-heading" class="text-sm font-semibold text-slate-800">Provenance</h2>
    <dl class="mt-3 space-y-2 text-sm">
      <div>
        <dt class="text-xs font-medium text-slate-500">Source page</dt>
        <dd class="break-words">
          <a v-if="sourceHref" :href="sourceHref" target="_blank" rel="noopener noreferrer nofollow" class="inline-flex items-start gap-1 text-brand-700 hover:underline" data-testid="source-link">
            <span class="break-all">{{ p.source_url }}</span><ExternalLink class="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden="true" />
          </a>
          <span v-else-if="p.source_url" class="break-all">{{ p.source_url }}</span>
          <span v-else class="text-slate-400">Entered manually</span>
        </dd>
      </div>
      <div v-if="p.canonical_url && p.canonical_url !== p.source_url">
        <dt class="text-xs font-medium text-slate-500">Canonical URL</dt>
        <dd class="break-all">
          <a v-if="canonicalHref" :href="canonicalHref" target="_blank" rel="noopener noreferrer nofollow" class="text-brand-700 hover:underline">{{ p.canonical_url }}</a>
          <span v-else>{{ p.canonical_url }}</span>
        </dd>
      </div>
      <div class="grid grid-cols-2 gap-2">
        <div>
          <dt class="text-xs font-medium text-slate-500">Extraction method</dt>
          <dd>{{ p.extraction_method || '—' }}</dd>
        </div>
        <div>
          <dt class="text-xs font-medium text-slate-500">Overall confidence</dt>
          <dd>
            <template v-if="confidence !== null">
              {{ percent(confidence) }}
              <span class="mt-1 block h-1.5 w-full rounded-full bg-slate-100"><span class="block h-1.5 rounded-full" :class="tone(confidence)" :style="{ width: `${Math.round(confidence * 100)}%` }" /></span>
            </template>
            <span v-else class="text-slate-400">Not reported</span>
          </dd>
        </div>
        <div>
          <dt class="text-xs font-medium text-slate-500">First seen</dt>
          <dd :title="formatDateTime(p.first_seen_at)">{{ formatDateTime(p.first_seen_at) }}</dd>
        </div>
        <div>
          <dt class="text-xs font-medium text-slate-500">Last checked</dt>
          <dd :title="formatDateTime(p.last_checked_at)">{{ relativeTime(p.last_checked_at) }}</dd>
        </div>
      </div>
      <div v-if="fieldConfidence.length">
        <dt class="text-xs font-medium text-slate-500">Confidence per field</dt>
        <dd>
          <ul class="mt-1 space-y-1">
            <li v-for="[field, v] in fieldConfidence" :key="field" class="grid grid-cols-[7rem_1fr_3rem] items-center gap-2 text-xs">
              <span class="truncate text-slate-700">{{ titleCase(field) }}</span>
              <span class="h-1.5 rounded-full bg-slate-100"><span class="block h-1.5 rounded-full" :class="tone(v)" :style="{ width: `${Math.round(Math.min(1, Math.max(0, v)) * 100)}%` }" /></span>
              <span class="text-right text-slate-500">{{ percent(v) }}</span>
            </li>
          </ul>
        </dd>
      </div>
      <div v-if="evidence.length">
        <dt class="text-xs font-medium text-slate-500">Evidence</dt>
        <dd>
          <dl class="mt-1 space-y-1">
            <div v-for="[field, ex] in evidence" :key="field" class="text-xs">
              <dt class="font-medium text-slate-700">{{ titleCase(field) }}</dt>
              <dd><code class="block rounded bg-slate-50 px-1.5 py-1 break-words whitespace-pre-wrap text-slate-700">{{ ex }}</code></dd>
            </div>
          </dl>
        </dd>
      </div>
      <div v-if="p.captured_page">
        <dt class="text-xs font-medium text-slate-500">Captured page</dt>
        <dd>
          <RouterLink :to="`/captures/${p.captured_page.id}`" class="text-brand-700 hover:underline" data-testid="captured-page-link">{{ p.captured_page.title || p.captured_page.url }}</RouterLink>
          <span class="block text-xs text-slate-500">{{ p.captured_page.adapter_key }} · {{ p.captured_page.extraction_method }}{{ p.captured_page.has_snapshot ? ' · HTML snapshot stored' : '' }}</span>
          <ul v-if="p.captured_page.warnings.length" class="mt-1 list-disc pl-4 text-xs text-amber-800">
            <li v-for="w in p.captured_page.warnings" :key="w">{{ w }}</li>
          </ul>
        </dd>
      </div>
      <div v-else-if="p.captured_page_id">
        <dt class="text-xs font-medium text-slate-500">Captured page</dt>
        <dd><RouterLink :to="`/captures/${p.captured_page_id}`" class="text-brand-700 hover:underline">Open captured page</RouterLink></dd>
      </div>
      <div>
        <dt class="text-xs font-medium text-slate-500">Import history</dt>
        <dd>
          <p v-if="imports.length === 0" class="text-slate-400">No imports; created by hand.</p>
          <ul v-else class="mt-1 divide-y divide-slate-100 rounded-md border border-slate-200" data-testid="import-history">
            <li v-for="imp in imports" :key="imp.id" class="flex flex-wrap items-center gap-2 px-2 py-1.5 text-xs">
              <RouterLink :to="`/imports/${imp.id}`" class="text-brand-700 hover:underline">{{ formatDateTime(imp.created_at) }}</RouterLink>
              <UiBadge :value="imp.status">{{ IMPORT_STATUS_LABELS[imp.status] ?? imp.status }}</UiBadge>
              <span class="text-slate-500">{{ imp.source }}{{ imp.decision ? ` · ${DECISION_LABELS[imp.decision] ?? imp.decision}` : '' }}</span>
              <span v-if="imp.product_id !== p.id && imp.duplicate_of_id === p.id" class="text-slate-500">· matched as duplicate</span>
            </li>
          </ul>
        </dd>
      </div>
    </dl>
  </section>
</template>
