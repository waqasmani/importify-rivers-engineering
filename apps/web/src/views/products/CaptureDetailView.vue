<script setup lang="ts">
// One captured page: metadata, what the extractor found, structured data, the user's selections,
// warnings and the sanitized snapshot, with actions to import it as a product or analyze it.
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { ExternalLink, PackagePlus, ScanSearch, Trash2 } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import StateViews from '@/components/ui/StateViews.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiButton from '@/components/ui/UiButton.vue'
import RecordPanels from '@/components/record/RecordPanels.vue'
import JsonView from '@/components/products/JsonView.vue'
import SnapshotViewer from '@/components/products/SnapshotViewer.vue'
import { useAuth } from '@/stores/auth'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { useDeviceNames } from '@/components/products/useDeviceNames'
import { draftFromExtracted, asRecord, safeHref, str, mapFromRows } from '@/components/products/product-utils'
import { formatDateTime, formatMoney, percent, relativeTime, titleCase } from '@/lib/format'

const props = defineProps<{ id: string }>()
const auth = useAuth()
const router = useRouter()
const qc = useQueryClient()
const toast = useToast()
const { confirm } = useConfirm()
const { deviceName } = useDeviceNames()

const q = useQuery({
  queryKey: computed(() => ['capture', props.id]),
  queryFn: ({ signal }) => api.get<Schemas['CaptureDetailDoc']>(`/captures/${props.id}`, undefined, signal),
})
const cap = computed(() => q.data.value?.capture ?? null)

const extracted = computed(() => asRecord(cap.value?.extracted))
const hasExtracted = computed(() => Object.keys(extracted.value).length > 0)
const fields = computed(() => {
  if (!cap.value || !hasExtracted.value) return null
  const d = draftFromExtracted(cap.value.extracted)
  const amount = d.price_amount || ''
  const price = amount ? formatMoney(amount, d.currency || null) : d.price ? `${d.price}${d.currency ? ` (${d.currency})` : ''}` : ''
  const rows: [string, string][] = [
    ['Title', d.title],
    ['Price', price],
    ['Brand', d.brand],
    ['Model', d.model],
    ['SKU', d.sku],
    ['MPN', d.mpn],
    ['GTIN', d.gtin],
    ['Category', d.category],
    ['Availability', str(extracted.value.availability)],
    ['Supplier', [d.supplier_name, d.supplier_url].filter(Boolean).join(' · ')],
    ['Extraction method', [d.extraction_method !== 'manual' ? d.extraction_method : '', d.adapter_key].filter(Boolean).join(' / ')],
    ['Confidence', d.confidence === null ? '' : percent(d.confidence)],
    ['Variants', d.variants.length ? `${d.variants.length}` : ''],
  ]
  return {
    rows: rows.filter(([, v]) => v),
    description: d.description,
    images: d.images.map((i) => i.url).filter((u) => !!safeHref(u)).slice(0, 12),
    specs: Object.entries(mapFromRows(d.specifications)),
    evidence: Object.entries(d.evidence),
    fieldConfidence: Object.entries(d.field_confidence),
  }
})

// JSON-LD and microdata first; the database does not keep the original key order.
function structuredRank(k: string): number {
  const l = k.toLowerCase()
  if (/json.?ld|ld\+json/.test(l)) return 0
  if (l.includes('microdata')) return 1
  if (l.includes('opengraph') || l === 'og') return 2
  return 3
}
function structuredLabel(k: string): string {
  const l = k.toLowerCase()
  if (/^(json.?ld|ld\+json)$/.test(l)) return 'JSON-LD'
  if (l === 'opengraph' || l === 'og') return 'Open Graph'
  return titleCase(k)
}
const structured = computed<[string, unknown][]>(() => {
  const sd = cap.value?.structured_data
  if (Array.isArray(sd)) return sd.length ? [['Structured data', sd]] : []
  return Object.entries(asRecord(sd)).sort((a, b) => structuredRank(a[0]) - structuredRank(b[0]) || a[0].localeCompare(b[0]))
})

interface Sel {
  kind: string
  text: string
  selector: string
  rows: string[][]
  src: string
  alt: string
  label: string
}
const selections = computed<Sel[]>(() => {
  const raw = cap.value?.selections
  if (!Array.isArray(raw)) return []
  return raw.map((x) => {
    const r = asRecord(x)
    return {
      kind: str(r.kind) || 'text',
      text: str(r.text),
      selector: str(r.selector),
      rows: Array.isArray(r.rows) ? r.rows.map((row) => (Array.isArray(row) ? row.map((c) => str(c)) : [])) : [],
      src: str(r.src),
      alt: str(r.alt),
      label: str(r.label),
    }
  })
})

async function remove() {
  if (!cap.value) return
  const ok = await confirm({
    title: 'Delete captured page',
    message: `Delete the capture of “${cap.value.title || cap.value.url}” with its selections, snapshot, notes and files? Products imported from it are kept. This cannot be undone.`,
    confirmLabel: 'Delete',
    danger: true,
  })
  if (!ok) return
  try {
    await api.del(`/captures/${props.id}`)
    toast.success('Captured page deleted.')
    void qc.invalidateQueries({ queryKey: ['captures'] })
    qc.removeQueries({ queryKey: ['capture', props.id] })
    await router.replace('/captures')
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
</script>

<template>
  <div>
    <StateViews v-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <template v-else-if="cap">
      <PageHeader :title="cap.title || cap.url" back="/captures">
        <template #meta>
          <div class="mt-2 flex flex-wrap items-center gap-2 text-sm text-slate-600">
            <UiBadge :value="cap.page_type" :tone="cap.page_type === 'product' ? 'blue' : 'slate'" />
            <span>{{ cap.domain }}</span>
            <span>· captured <time :datetime="cap.captured_at" :title="formatDateTime(cap.captured_at)">{{ relativeTime(cap.captured_at) }}</time></span>
            <span v-if="cap.device_id">· {{ deviceName(cap.device_id) }}</span>
          </div>
        </template>
        <template #actions>
          <RouterLink v-if="auth.can('products.write')" :to="{ path: '/imports/new', query: { capture: cap.id } }" class="inline-flex items-center gap-1.5 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700" data-testid="import-capture">
            <PackagePlus class="h-4 w-4" aria-hidden="true" /> Import as product
          </RouterLink>
          <RouterLink v-if="auth.can('analysis.write')" :to="{ path: '/analysis/new', query: { capture: cap.id } }" class="inline-flex items-center gap-1.5 rounded-md border border-slate-300 bg-white px-3.5 py-2 text-sm font-medium text-slate-700 shadow-xs hover:bg-slate-50" data-testid="analyze-capture">
            <ScanSearch class="h-4 w-4" aria-hidden="true" /> Analyze this page
          </RouterLink>
          <UiButton v-if="auth.can('products.delete')" variant="danger" @click="remove"><Trash2 class="h-4 w-4" /> Delete</UiButton>
        </template>
      </PageHeader>

      <div class="grid gap-6 lg:grid-cols-3">
        <div class="min-w-0 space-y-6 lg:col-span-2">
          <section class="card p-4" aria-labelledby="extracted-heading" data-testid="extracted">
            <h2 id="extracted-heading" class="text-sm font-semibold text-slate-800">Extracted fields</h2>
            <p v-if="!fields" class="mt-2 text-sm text-slate-500">The extractor found no product fields on this page.</p>
            <template v-else>
              <dl class="mt-3 grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2">
                <div v-for="[k, v] in fields.rows" :key="k">
                  <dt class="text-xs font-medium text-slate-500">{{ k }}</dt>
                  <dd class="break-words text-slate-800">{{ v }}</dd>
                </div>
              </dl>
              <p v-if="fields.description" class="mt-3 text-sm whitespace-pre-wrap text-slate-700">{{ fields.description }}</p>
              <div v-if="fields.images.length" class="mt-3 flex flex-wrap gap-2" aria-label="Extracted images">
                <a v-for="u in fields.images" :key="u" :href="safeHref(u)" target="_blank" rel="noopener noreferrer nofollow" class="block h-16 w-16 overflow-hidden rounded border border-slate-200">
                  <img :src="u" alt="Extracted product image" loading="lazy" referrerpolicy="no-referrer" class="h-full w-full object-cover" />
                </a>
              </div>
              <table v-if="fields.specs.length" class="mt-3 min-w-full text-sm">
                <caption class="mb-1 text-left text-xs font-medium text-slate-500">Specifications</caption>
                <tbody class="divide-y divide-slate-100">
                  <tr v-for="[k, v] in fields.specs" :key="k">
                    <th scope="row" class="w-1/3 py-1 pr-3 text-left font-medium text-slate-600">{{ k }}</th>
                    <td class="py-1 break-words">{{ v }}</td>
                  </tr>
                </tbody>
              </table>
              <div v-if="fields.evidence.length || fields.fieldConfidence.length" class="mt-3 grid gap-3 sm:grid-cols-2">
                <dl v-if="fields.evidence.length" class="space-y-1 text-xs">
                  <dt class="font-medium text-slate-500">Evidence</dt>
                  <dd v-for="[k, v] in fields.evidence" :key="k"><span class="font-medium text-slate-700">{{ titleCase(k) }}:</span> <code class="break-words">{{ v }}</code></dd>
                </dl>
                <dl v-if="fields.fieldConfidence.length" class="space-y-1 text-xs">
                  <dt class="font-medium text-slate-500">Confidence per field</dt>
                  <dd v-for="[k, v] in fields.fieldConfidence" :key="k">{{ titleCase(k) }}: {{ percent(v) }}</dd>
                </dl>
              </div>
            </template>
            <JsonView v-if="hasExtracted" :value="cap.extracted" summary="Extracted data (JSON)" class="mt-3" />
          </section>

          <section class="card p-4" aria-labelledby="selections-heading" data-testid="selections">
            <h2 id="selections-heading" class="text-sm font-semibold text-slate-800">Your selections <span class="font-normal text-slate-500">({{ selections.length }})</span></h2>
            <p v-if="selections.length === 0" class="mt-2 text-sm text-slate-500">No text, tables or images were selected on the page.</p>
            <ul v-else class="mt-3 space-y-3">
              <li v-for="(s, i) in selections" :key="i" class="rounded-md border border-slate-200 p-3">
                <div class="mb-1 flex flex-wrap items-center gap-2 text-xs text-slate-500">
                  <UiBadge tone="slate">{{ titleCase(s.kind) }}</UiBadge>
                  <span v-if="s.label" class="font-medium text-slate-700">{{ s.label }}</span>
                  <code v-if="s.selector" class="truncate">{{ s.selector }}</code>
                </div>
                <div v-if="s.kind === 'table' && s.rows.length" class="overflow-x-auto">
                  <table class="min-w-full border border-slate-200 text-xs">
                    <tbody>
                      <tr v-for="(row, ri) in s.rows.slice(0, 100)" :key="ri" class="border-b border-slate-100">
                        <td v-for="(c, ci) in row" :key="ci" class="border-r border-slate-100 px-2 py-1 align-top">{{ c }}</td>
                      </tr>
                    </tbody>
                  </table>
                  <p v-if="s.rows.length > 100" class="mt-1 text-xs text-slate-500">{{ s.rows.length - 100 }} more rows not shown.</p>
                </div>
                <div v-else-if="s.kind === 'image' && safeHref(s.src)" class="flex items-start gap-3">
                  <img :src="s.src" :alt="s.alt || 'Selected image'" loading="lazy" referrerpolicy="no-referrer" class="h-20 w-20 rounded border border-slate-200 object-cover" />
                  <a :href="safeHref(s.src)" target="_blank" rel="noopener noreferrer nofollow" class="text-xs break-all text-brand-700 hover:underline">{{ s.src }}</a>
                </div>
                <a v-else-if="s.kind === 'link' && safeHref(s.src || s.text)" :href="safeHref(s.src || s.text)" target="_blank" rel="noopener noreferrer nofollow" class="text-sm break-all text-brand-700 hover:underline">{{ s.text || s.src }}</a>
                <p v-if="s.text && s.kind !== 'link'" class="max-h-64 overflow-y-auto text-sm whitespace-pre-wrap text-slate-800">{{ s.text }}</p>
              </li>
            </ul>
          </section>

          <section class="card space-y-2 p-4" aria-labelledby="structured-heading" data-testid="structured-data">
            <h2 id="structured-heading" class="text-sm font-semibold text-slate-800">Structured data</h2>
            <p class="text-xs text-slate-500">JSON-LD, microdata, Open Graph and meta tags found on the page, as captured.</p>
            <p v-if="structured.length === 0" class="text-sm text-slate-500">No structured data was found.</p>
            <JsonView v-for="([k, v], i) in structured" :key="k" :value="v" :summary="structuredLabel(k)" :open="i === 0" />
          </section>

          <SnapshotViewer :capture-id="cap.id" :has-snapshot="q.data.value?.has_snapshot ?? false" />
        </div>

        <div class="min-w-0 space-y-6">
          <section class="card p-4" aria-labelledby="meta-heading" data-testid="capture-meta">
            <h2 id="meta-heading" class="text-sm font-semibold text-slate-800">Page</h2>
            <dl class="mt-3 space-y-2 text-sm">
              <div>
                <dt class="text-xs font-medium text-slate-500">URL</dt>
                <dd>
                  <a v-if="safeHref(cap.url)" :href="safeHref(cap.url)" target="_blank" rel="noopener noreferrer nofollow" class="inline-flex items-start gap-1 break-all text-brand-700 hover:underline">{{ cap.url }}<ExternalLink class="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden="true" /></a>
                  <span v-else class="break-all">{{ cap.url }}</span>
                </dd>
              </div>
              <div v-if="cap.canonical_url && cap.canonical_url !== cap.url">
                <dt class="text-xs font-medium text-slate-500">Canonical URL</dt>
                <dd class="break-all">{{ cap.canonical_url }}</dd>
              </div>
              <div v-if="cap.description">
                <dt class="text-xs font-medium text-slate-500">Description</dt>
                <dd class="text-slate-700">{{ cap.description }}</dd>
              </div>
              <div class="grid grid-cols-2 gap-2">
                <div><dt class="text-xs font-medium text-slate-500">Page type</dt><dd>{{ titleCase(cap.page_type) }}</dd></div>
                <div><dt class="text-xs font-medium text-slate-500">Adapter</dt><dd class="break-all">{{ cap.adapter_key }}</dd></div>
                <div><dt class="text-xs font-medium text-slate-500">Extraction method</dt><dd class="break-all">{{ cap.extraction_method }}</dd></div>
                <div><dt class="text-xs font-medium text-slate-500">Captured</dt><dd>{{ formatDateTime(cap.captured_at) }}</dd></div>
                <div><dt class="text-xs font-medium text-slate-500">Captured by</dt><dd>{{ cap.captured_by && cap.captured_by === auth.user?.id ? 'You' : cap.captured_by ? 'Another user' : '—' }}</dd></div>
                <div><dt class="text-xs font-medium text-slate-500">Device</dt><dd>{{ deviceName(cap.device_id) || 'Web or API' }}</dd></div>
              </div>
              <div>
                <dt class="text-xs font-medium text-slate-500">Content hash (SHA-256)</dt>
                <dd class="font-mono text-xs break-all text-slate-600">{{ cap.content_sha256 || '—' }}</dd>
              </div>
            </dl>
          </section>

          <section class="card p-4" aria-labelledby="warn-heading" data-testid="capture-warnings">
            <h2 id="warn-heading" class="text-sm font-semibold text-slate-800">Warnings</h2>
            <p v-if="cap.warnings.length === 0" class="mt-2 text-sm text-slate-500">No warnings.</p>
            <ul v-else class="mt-2 list-disc space-y-1 pl-5 text-sm text-amber-900">
              <li v-for="w in cap.warnings" :key="w">{{ w }}</li>
            </ul>
          </section>
        </div>
      </div>

      <div class="mt-6">
        <RecordPanels entity-type="captured_page" :entity-id="cap.id" />
      </div>
    </template>
  </div>
</template>
