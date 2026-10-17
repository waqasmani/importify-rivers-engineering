<script setup lang="ts">
// Side-by-side view of what was submitted and what REA normalized it to. Values that changed during
// normalization are highlighted so reviewers can see what the server altered or dropped.
import { computed } from 'vue'
import { formatMoney, percent, titleCase } from '@/lib/format'
import { asRecord, normalizeAvailability, str, stringMap } from './product-utils'

const props = defineProps<{ payload: unknown; normalized: unknown }>()

interface Row {
  field: string
  submitted: string
  normalized: string
  /** REA altered or dropped what was submitted (not merely reformatted or derived). */
  changed?: boolean
  list?: string[]
  images?: string[]
}

function clip(s: string, n = 240): string {
  return s.length > n ? `${s.slice(0, n)}…` : s
}
function count(v: unknown, noun: string): string {
  const n = Array.isArray(v) ? v.length : Object.keys(asRecord(v)).length
  return n === 0 ? '' : `${n} ${noun}${n === 1 ? '' : 's'}`
}

const rows = computed<Row[]>(() => {
  const p = asRecord(props.payload)
  const n = asRecord(props.normalized)
  const ids = stringMap(n.identifiers)
  const nVariants = Array.isArray(n.variants) ? n.variants.map((x) => asRecord(x)) : []
  const nImages = Array.isArray(n.images) ? n.images.map((x) => str(asRecord(x).url)).filter(Boolean) : []
  const nSpecs = stringMap(n.specifications)
  const submittedPrice = str(p.price_amount) ? `${str(p.price_amount)} ${str(p.currency)}`.trim() : `${str(p.price)}${str(p.currency) ? ` (${str(p.currency)})` : ''}`
  const same = (a: string, b: string) => a.trim().toLowerCase() === b.trim().toLowerCase()
  const altered = (sub: string, norm: string) => sub.trim() !== '' && !same(sub, norm)
  const nCount = (v: unknown) => (Array.isArray(v) ? v.length : Object.keys(asRecord(v)).length)
  const subPriceAmount = str(p.price_amount)
  const out: Row[] = [
    { field: 'Title', submitted: str(p.title), normalized: str(n.title), changed: altered(str(p.title), str(n.title)) },
    { field: 'URL', submitted: str(p.url), normalized: str(n.source_url), changed: altered(str(p.url), str(n.source_url)) },
    { field: 'Canonical URL', submitted: str(p.canonical_url), normalized: str(n.canonical_url), changed: altered(str(p.canonical_url), str(n.canonical_url)) },
    { field: 'Source domain', submitted: '', normalized: str(n.source_domain) },
    {
      field: 'Price',
      submitted: submittedPrice,
      normalized: str(n.price) ? formatMoney(str(n.price), str(n.price_currency) || null) : '',
      changed: (submittedPrice.trim() !== '' && !str(n.price)) || (subPriceAmount !== '' && Number(subPriceAmount) !== Number(str(n.price))),
    },
    { field: 'Brand', submitted: str(p.brand), normalized: str(n.brand), changed: altered(str(p.brand), str(n.brand)) },
    { field: 'SKU', submitted: str(p.sku), normalized: str(n.sku), changed: altered(str(p.sku), str(n.sku)) },
    { field: 'Model', submitted: str(p.model), normalized: str(n.model), changed: altered(str(p.model), str(n.model)) },
    { field: 'MPN', submitted: str(p.mpn), normalized: str(n.mpn), changed: altered(str(p.mpn), str(n.mpn)) },
    {
      field: 'GTIN',
      submitted: str(p.gtin),
      normalized: str(n.gtin) || (ids.gtin_unverified ? `${ids.gtin_unverified} (unverified)` : ''),
      changed: str(p.gtin).trim() !== '' && str(p.gtin).replace(/\D/g, '') !== str(n.gtin),
    },
    {
      field: 'Availability',
      submitted: str(p.availability),
      normalized: titleCase(str(n.availability)),
      changed: str(p.availability).trim() !== '' && normalizeAvailability(str(p.availability)) !== str(n.availability),
    },
    { field: 'Category', submitted: str(p.category), normalized: str(n.category), changed: altered(str(p.category), str(n.category)) },
    { field: 'Description', submitted: clip(str(p.description)), normalized: clip(str(n.description)), changed: altered(str(p.description).replace(/\s+/g, ' '), str(n.description).replace(/\s+/g, ' ')) },
    {
      field: 'Specifications',
      submitted: count(p.specifications, 'row'),
      normalized: count(nSpecs, 'row'),
      changed: nCount(p.specifications) !== Object.keys(nSpecs).length,
      list: Object.entries(nSpecs).map(([k, v]) => `${k}: ${v}`),
    },
    {
      field: 'Variants',
      submitted: count(p.variants, 'variant'),
      normalized: count(nVariants, 'variant'),
      changed: nCount(p.variants) !== nVariants.length,
      list: nVariants.map((v) => [str(v.title) || str(v.sku), str(v.price) ? formatMoney(str(v.price), str(v.currency) || null) : '', titleCase(str(v.availability))].filter(Boolean).join(' · ')),
    },
    { field: 'Images', submitted: count(p.images, 'image'), normalized: count(nImages, 'image'), changed: nCount(p.images) !== nImages.length, images: nImages.slice(0, 8) },
    {
      field: 'Other identifiers',
      submitted: Object.entries(stringMap(p.identifiers)).map(([k, v]) => `${k}: ${v}`).join(', '),
      normalized: Object.entries(ids).map(([k, v]) => `${k}: ${v}`).join(', '),
    },
    {
      field: 'Supplier',
      submitted: [str(p.supplier_name), str(p.supplier_url)].filter(Boolean).join(' · '),
      normalized: [str(n.supplier_name), str(n.supplier_domain)].filter(Boolean).join(' · '),
      changed: altered(str(p.supplier_name), str(n.supplier_name)) || (str(p.supplier_url) !== '' && !str(n.supplier_domain)),
    },
    { field: 'Extraction method', submitted: [str(p.extraction_method), str(p.adapter_key)].filter(Boolean).join(' / '), normalized: [str(n.extraction_method), str(n.adapter_key)].filter(Boolean).join(' / ') },
    {
      field: 'Confidence',
      submitted: p.confidence === null || p.confidence === undefined ? '' : percent(p.confidence as number),
      normalized: n.confidence === null || n.confidence === undefined ? '' : percent(str(n.confidence)),
    },
  ]
  return out.filter((r) => r.submitted || r.normalized)
})

</script>

<template>
  <div class="overflow-hidden rounded-md border border-slate-200 bg-white" data-testid="payload-comparison">
    <div class="hidden grid-cols-[10rem_1fr_1fr] gap-3 border-b border-slate-200 bg-slate-50 px-3 py-2 text-xs font-semibold tracking-wide text-slate-500 uppercase sm:grid">
      <span>Field</span><span>Submitted</span><span>Normalized by REA</span>
    </div>
    <dl class="divide-y divide-slate-100">
      <div v-for="r in rows" :key="r.field" class="grid gap-1 px-3 py-2 text-sm sm:grid-cols-[10rem_1fr_1fr] sm:gap-3" :class="r.changed ? 'bg-amber-50/60' : ''">
        <dt class="font-medium text-slate-600">
          {{ r.field }}
          <span v-if="r.changed" class="ml-1 text-xs font-normal text-amber-700">changed</span>
        </dt>
        <dd class="min-w-0 break-words text-slate-500">
          <span class="text-xs text-slate-400 sm:hidden">Submitted: </span>{{ r.submitted || '—' }}
        </dd>
        <dd class="min-w-0 break-words text-slate-900">
          <span class="text-xs text-slate-400 sm:hidden">Normalized: </span>{{ r.normalized || '—' }}
          <ul v-if="r.list?.length" class="mt-1 list-disc space-y-0.5 pl-5 text-xs text-slate-600">
            <li v-for="(x, i) in r.list.slice(0, 30)" :key="i">{{ x }}</li>
            <li v-if="r.list.length > 30" class="list-none text-slate-400">and {{ r.list.length - 30 }} more</li>
          </ul>
          <div v-if="r.images?.length" class="mt-1 flex flex-wrap gap-1.5">
            <img v-for="u in r.images" :key="u" :src="u" alt="" loading="lazy" referrerpolicy="no-referrer" class="h-12 w-12 rounded border border-slate-200 object-cover" />
          </div>
        </dd>
      </div>
    </dl>
  </div>
</template>
