<script setup lang="ts">
// Shows what the extractor read from a saved page: merged fields with confidence and evidence,
// images (as links, never loaded), variants, specifications, metadata, warnings and each adapter's
// own result. Everything is rendered as text.
import { computed } from 'vue'
import { FIELD_KEYS, METADATA_KEYS, type AdapterFields } from '@rea/extractor'
import UiBadge from '@/components/ui/UiBadge.vue'
import { fieldLabel } from './marketplaces'
import { displayValue, type LabRun } from './lab'

const props = defineProps<{ run: LabRun }>()

const page = computed(() => props.run.page)
const payload = computed(() => page.value.product.payload)

function payloadValue(key: string): string {
  const p = payload.value as unknown as Record<string, unknown>
  if (key === 'price') return p['price'] ? `${String(p['price'])}${p['price_amount'] ? ` (${String(p['price_amount'])})` : ''}` : ''
  return displayValue(p[key])
}

const fieldRows = computed(() =>
  FIELD_KEYS.map((key) => ({ key, value: payloadValue(key), info: page.value.product.fields[key] })).filter((r) => r.value || r.info),
)
const metadataRows = computed(() => METADATA_KEYS.map((key) => ({ key, m: page.value.product.metadata[key] })).filter((r) => r.m))
const focusRows = computed(() => {
  const f = props.run.focus?.output.fields as AdapterFields | undefined
  if (!f) return []
  return Object.entries(f)
    .filter(([, v]) => v)
    .map(([key, v]) => ({ key, value: displayValue(v!.value), confidence: v!.confidence, evidence: v!.evidence, warnings: v!.warnings ?? [] }))
})
const pct = (n?: number | null) => (n === null || n === undefined ? '' : `${Math.round(n * 100)}%`)
</script>

<template>
  <div class="space-y-6" data-testid="lab-results">
    <section class="card p-4" aria-labelledby="lab-summary">
      <h2 id="lab-summary" class="mb-2 text-sm font-semibold text-slate-800">Summary</h2>
      <dl class="grid grid-cols-1 gap-x-6 gap-y-1 text-sm sm:grid-cols-[10rem_1fr]">
        <dt class="text-slate-500">Page type</dt>
        <dd data-testid="lab-page-type">{{ page.pageType }} <span class="text-xs text-slate-500">({{ page.pageTypeReason }})</span></dd>
        <dt class="text-slate-500">Primary adapter</dt>
        <dd><code>{{ payload.adapter_key }}</code> <span class="text-xs text-slate-500">method {{ payload.extraction_method }}</span></dd>
        <dt class="text-slate-500">Overall confidence</dt>
        <dd>{{ pct(payload.confidence) || 'none (no title)' }}</dd>
      </dl>
      <ul v-if="run.notes.length" class="mt-3 list-disc space-y-1 pl-5 text-sm text-amber-800">
        <li v-for="n in run.notes" :key="n">{{ n }}</li>
      </ul>
    </section>

    <section v-if="run.focus" class="card overflow-x-auto" aria-labelledby="lab-focus">
      <h2 id="lab-focus" class="px-4 pt-3 text-sm font-semibold text-slate-800">
        What {{ run.focus.cfg.name }} found
        <UiBadge :tone="run.focus.output.matched ? 'green' : 'red'" class="ml-1">{{ run.focus.output.matched ? 'product signal found' : 'product signal not found' }}</UiBadge>
      </h2>
      <p v-if="focusRows.length === 0" class="px-4 py-3 text-sm text-slate-500">This adapter produced no fields for the page.</p>
      <table v-else class="mt-2 w-full text-left text-sm" data-testid="lab-focus-table">
        <thead class="bg-slate-50 text-xs text-slate-500"><tr><th class="px-4 py-2">Field</th><th class="px-4 py-2">Value</th><th class="px-4 py-2">Confidence</th><th class="px-4 py-2">Evidence</th></tr></thead>
        <tbody class="divide-y divide-slate-100">
          <tr v-for="r in focusRows" :key="r.key" class="align-top">
            <td class="px-4 py-2 font-medium whitespace-nowrap">{{ fieldLabel(r.key) }}</td>
            <td class="max-w-md px-4 py-2 break-words">{{ r.value }}<p v-for="w in r.warnings" :key="w" class="text-xs text-amber-700">{{ w }}</p></td>
            <td class="px-4 py-2">{{ pct(r.confidence) }}</td>
            <td class="px-4 py-2 font-mono text-xs break-all text-slate-600">{{ r.evidence }}</td>
          </tr>
        </tbody>
      </table>
      <ul v-if="run.focus.output.warnings.length" class="list-disc space-y-1 px-8 py-3 text-xs text-amber-800">
        <li v-for="w in run.focus.output.warnings" :key="w">{{ w }}</li>
      </ul>
    </section>

    <section class="card overflow-x-auto" aria-labelledby="lab-fields">
      <h2 id="lab-fields" class="px-4 pt-3 text-sm font-semibold text-slate-800">Merged product fields (what an import would contain)</h2>
      <table class="mt-2 w-full text-left text-sm" data-testid="lab-fields">
        <thead class="bg-slate-50 text-xs text-slate-500"><tr><th class="px-4 py-2">Field</th><th class="px-4 py-2">Value</th><th class="px-4 py-2">Confidence</th><th class="px-4 py-2">Adapter</th><th class="px-4 py-2">Evidence</th></tr></thead>
        <tbody class="divide-y divide-slate-100">
          <tr v-for="r in fieldRows" :key="r.key" class="align-top" :data-testid="`lab-field-${r.key}`">
            <td class="px-4 py-2 font-medium whitespace-nowrap">{{ fieldLabel(r.key) }}</td>
            <td class="max-w-md px-4 py-2 break-words whitespace-pre-line">{{ r.value }}</td>
            <td class="px-4 py-2">{{ pct(r.info?.confidence) }}</td>
            <td class="px-4 py-2"><code class="text-xs">{{ r.info?.adapter }}</code></td>
            <td class="px-4 py-2 font-mono text-xs break-all text-slate-600">{{ r.info?.evidence }}</td>
          </tr>
        </tbody>
      </table>
    </section>

    <section v-if="payload.images.length" class="card p-4" aria-labelledby="lab-images">
      <h2 id="lab-images" class="mb-2 text-sm font-semibold text-slate-800">Images ({{ payload.images.length }})</h2>
      <p class="mb-2 text-xs text-slate-500">Shown as links: the lab never loads anything from the saved page.</p>
      <ol class="list-decimal space-y-0.5 pl-5 text-xs break-all">
        <li v-for="img in payload.images" :key="img.url"><a :href="img.url" target="_blank" rel="noopener noreferrer nofollow" class="text-brand-700 hover:underline">{{ img.url }}</a></li>
      </ol>
    </section>

    <section v-if="payload.variants.length" class="card overflow-x-auto" aria-labelledby="lab-variants">
      <h2 id="lab-variants" class="px-4 pt-3 text-sm font-semibold text-slate-800">Variants ({{ payload.variants.length }})</h2>
      <table class="mt-2 w-full text-left text-sm" data-testid="lab-variants">
        <thead class="bg-slate-50 text-xs text-slate-500"><tr><th class="px-4 py-2">Title</th><th class="px-4 py-2">SKU</th><th class="px-4 py-2">Options</th><th class="px-4 py-2">Price</th><th class="px-4 py-2">Availability</th><th class="px-4 py-2">Image</th></tr></thead>
        <tbody class="divide-y divide-slate-100">
          <tr v-for="(v, i) in payload.variants" :key="i" class="align-top">
            <td class="px-4 py-2">{{ v.title }}</td>
            <td class="px-4 py-2 font-mono text-xs">{{ v.sku }}</td>
            <td class="px-4 py-2 text-xs">{{ Object.entries(v.options).map(([k, x]) => `${k}: ${x}`).join(', ') }}</td>
            <td class="px-4 py-2 whitespace-nowrap">{{ v.price_amount ? `${v.price_amount} ${v.currency}` : '' }}</td>
            <td class="px-4 py-2">{{ v.availability }}</td>
            <td class="px-4 py-2 text-xs break-all"><a v-if="v.image_url" :href="v.image_url" target="_blank" rel="noopener noreferrer nofollow" class="text-brand-700 hover:underline">link</a></td>
          </tr>
        </tbody>
      </table>
    </section>

    <section v-if="Object.keys(payload.specifications).length" class="card p-4" aria-labelledby="lab-specs">
      <h2 id="lab-specs" class="mb-2 text-sm font-semibold text-slate-800">Specifications</h2>
      <dl class="grid grid-cols-1 gap-x-4 gap-y-0.5 text-sm sm:grid-cols-[14rem_1fr]">
        <template v-for="(val, k) in payload.specifications" :key="k">
          <dt class="text-slate-500">{{ k }}</dt>
          <dd class="break-words">{{ val }}</dd>
        </template>
      </dl>
    </section>

    <section v-if="metadataRows.length" class="card overflow-x-auto" aria-labelledby="lab-metadata">
      <h2 id="lab-metadata" class="px-4 pt-3 text-sm font-semibold text-slate-800">Offer metadata (kept with captures, not imported as product fields)</h2>
      <table class="mt-2 w-full text-left text-sm">
        <tbody class="divide-y divide-slate-100">
          <tr v-for="r in metadataRows" :key="r.key" class="align-top">
            <td class="px-4 py-2 font-medium whitespace-nowrap">{{ fieldLabel(r.key) }}</td>
            <td class="px-4 py-2">{{ r.m!.value }}</td>
            <td class="px-4 py-2">{{ pct(r.m!.confidence) }}</td>
            <td class="px-4 py-2 font-mono text-xs break-all text-slate-600">{{ r.m!.evidence }}</td>
          </tr>
        </tbody>
      </table>
    </section>

    <section v-if="page.warnings.length" class="card p-4" aria-labelledby="lab-warnings">
      <h2 id="lab-warnings" class="mb-2 text-sm font-semibold text-slate-800">Warnings</h2>
      <ul class="list-disc space-y-1 pl-5 text-sm text-amber-800" data-testid="lab-warnings">
        <li v-for="w in page.warnings" :key="w">{{ w }}</li>
      </ul>
    </section>

    <section class="card overflow-x-auto" aria-labelledby="lab-adapters">
      <h2 id="lab-adapters" class="px-4 pt-3 text-sm font-semibold text-slate-800">Adapters that ran</h2>
      <table class="mt-2 w-full text-left text-sm">
        <thead class="bg-slate-50 text-xs text-slate-500"><tr><th class="px-4 py-2">Adapter</th><th class="px-4 py-2">Recognised the page</th><th class="px-4 py-2">Fields</th><th class="px-4 py-2">Warnings</th></tr></thead>
        <tbody class="divide-y divide-slate-100">
          <tr v-for="a in page.product.adapters" :key="a.key" class="align-top">
            <td class="px-4 py-2">{{ a.name }} <code class="text-xs text-slate-500">{{ a.key }}</code></td>
            <td class="px-4 py-2">{{ a.matched ? 'yes' : 'no' }}</td>
            <td class="px-4 py-2 text-xs">{{ a.fields.map(fieldLabel).join(', ') }}</td>
            <td class="px-4 py-2 text-xs text-amber-800">{{ a.warnings.join(' · ') }}</td>
          </tr>
        </tbody>
      </table>
    </section>
  </div>
</template>
