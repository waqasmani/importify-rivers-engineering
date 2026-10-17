<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { PackagePlus, Plus, Search, Trash2 } from 'lucide-vue-next'
import type { ImportRequestMsg, UiError } from '../../shared/messages'
import { parsePrice } from '../../shared/price'
import type { FieldInfo, FieldKey, Payload, Schemas, Selection } from '../../shared/types'
import { asUiError, call } from '../bridge'
import { percent } from '../format'
import { app, can, extract, link, saveCapture } from '../store'
import ConfBadge from './ConfBadge.vue'
import ErrorNote from './ErrorNote.vue'
import ExtLink from './ExtLink.vue'
import Notice from './Notice.vue'
import SectionCard from './SectionCard.vue'
import Spinner from './Spinner.vue'

interface ImageRow { url: string; alt: string; keep: boolean; width: number | null; height: number | null }
interface SpecRow { key: string; value: string }

const TEXT_FIELDS = [
  { key: 'brand', label: 'Brand', max: 200 },
  { key: 'sku', label: 'SKU', max: 120 },
  { key: 'model', label: 'Model', max: 200 },
  { key: 'mpn', label: 'MPN', max: 120 },
  { key: 'gtin', label: 'GTIN / EAN / UPC', max: 40 },
  { key: 'category', label: 'Category', max: 300 },
  { key: 'availability', label: 'Availability', max: 100 },
] as const
type TextKey = (typeof TEXT_FIELDS)[number]['key']

const form = reactive({
  title: '', price: '', currency: '', description: '', brand: '', sku: '', model: '', mpn: '', gtin: '', category: '', availability: '',
  supplier_name: '', supplier_url: '', tags: '',
  images: [] as ImageRow[],
  specs: [] as SpecRow[],
})
const edited = reactive(new Set<string>())
const picked = reactive<Record<string, string>>({})
const applied = new Set<string>()
const loadedFrom = ref('')

const x = computed(() => app.extraction)
const base = computed<Payload | null>(() => x.value?.product.payload ?? null)

function init(): void {
  const p = base.value
  if (!p) return
  loadedFrom.value = x.value?.extractedAt ?? ''
  form.title = p.title
  form.price = p.price ?? ''
  form.currency = p.currency
  form.description = p.description
  form.brand = p.brand
  form.sku = p.sku
  form.model = p.model
  form.mpn = p.mpn
  form.gtin = p.gtin
  form.category = p.category
  form.availability = p.availability
  form.supplier_name = p.supplier_name
  form.supplier_url = p.supplier_url
  form.images = p.images.map((i) => ({ ...i, keep: true }))
  form.specs = Object.entries(p.specifications).map(([key, value]) => ({ key, value }))
  edited.clear()
  applied.clear()
  for (const k of Object.keys(picked)) delete picked[k]
  check.value = null
  outcome.value = null
  applySelections(app.selections)
}

function applySelections(list: Selection[]): void {
  for (const s of list) {
    const id = `${s.label}|${s.selector}|${s.src ?? ''}|${s.text.slice(0, 80)}`
    if (applied.has(id)) continue
    applied.add(id)
    check.value = null
    const ev = `picked by user: ${s.selector || 'context menu'}`.slice(0, 1000)
    if (s.label === 'title' && s.text) { form.title = s.text.slice(0, 500); picked['title'] = ev }
    if (s.label === 'price' && s.text) {
      form.price = s.text.slice(0, 200)
      const pp = parsePrice(s.text)
      if (pp?.currency) form.currency = pp.currency
      picked['price'] = ev
    }
    if (s.label === 'description' && s.text) { form.description = s.text.slice(0, 50000); picked['description'] = ev }
    if (s.label === 'image' && s.src && !form.images.some((i) => i.url === s.src)) {
      form.images.unshift({ url: s.src, alt: s.alt ?? '', keep: true, width: null, height: null })
      picked['images'] = ev
    }
    if (s.label === 'specifications' && s.rows) {
      for (const r of s.rows) {
        const key = (r[0] ?? '').replace(/:\s*$/, '').trim()
        const value = r.slice(1).join(' ').trim()
        if (key && value && !form.specs.some((sp) => sp.key === key)) form.specs.push({ key: key.slice(0, 120), value: value.slice(0, 2000) })
      }
      picked['specifications'] = ev
    }
  }
}


function info(key: FieldKey): FieldInfo | { confidence: number; evidence: string } | undefined {
  if (picked[key]) return { confidence: 1, evidence: picked[key]! }
  if (edited.has(key)) return { confidence: 1, evidence: 'entered by the user in the extension' }
  return x.value?.product.fields[key]
}

function touch(key: string): void {
  edited.add(key)
  check.value = null
}

const parsedPrice = computed(() => (form.price.trim() ? parsePrice(form.price, form.currency.trim().toUpperCase()) : null))

function payload(): Payload {
  const p = base.value!
  const fieldConfidence: Record<string, number> = { ...p.field_confidence }
  const evidence: Record<string, string> = { ...p.evidence }
  for (const k of [...edited, ...Object.keys(picked)]) {
    fieldConfidence[k] = 1
    evidence[k] = picked[k] ?? 'entered by the user in the extension'
  }
  const specs: Record<string, string> = {}
  for (const s of form.specs) if (s.key.trim() && s.value.trim()) specs[s.key.trim().slice(0, 120)] = s.value.trim().slice(0, 2000)
  const price = form.price.trim()
  return {
    ...p,
    title: form.title.trim(),
    description: form.description.trim(),
    brand: form.brand.trim(), sku: form.sku.trim(), model: form.model.trim(), mpn: form.mpn.trim(), gtin: form.gtin.trim(),
    category: form.category.trim(), availability: form.availability.trim(),
    price: price || null,
    price_amount: price ? (parsedPrice.value?.amount ?? null) : null,
    currency: (form.currency.trim() || parsedPrice.value?.currency || '').toUpperCase().slice(0, 10),
    images: form.images.filter((i) => i.keep).slice(0, 40).map(({ url, alt, width, height }) => ({ url, alt, width, height })),
    specifications: specs,
    supplier_name: form.supplier_name.trim(),
    supplier_url: form.supplier_url.trim(),
    field_confidence: Object.fromEntries(Object.entries(fieldConfidence).slice(0, 40)),
    evidence: Object.fromEntries(Object.entries(evidence).slice(0, 40)),
  }
}

const check = ref<Schemas['CheckResponse'] | null>(null)
const checking = ref(false)
const error = ref<UiError | null>(null)
const decision = ref<'' | 'create_new' | 'update_existing' | 'skip'>('create_new')
const targetId = ref<string>('')
const supplierChoice = ref<'auto' | 'suggested' | 'chosen' | 'create'>('auto')
const chosenSupplier = ref<Schemas['SupplierView'] | null>(null)
const supplierQuery = ref('')
const supplierResults = ref<Schemas['SupplierView'][]>([])
const findingSuppliers = ref(false)
const withCapture = ref(true)
const submitting = ref(false)
const outcome = ref<{ httpStatus: number; outcome: Schemas['ImportOutcome'] } | null>(null)

const suggested = computed(() => (check.value?.suggested_supplier ?? null) as Schemas['DuplicateCandidate'] | null)

function request(): ImportRequestMsg {
  let supplierId: string | null = null
  if (supplierChoice.value === 'suggested' && suggested.value) supplierId = suggested.value.id
  if (supplierChoice.value === 'chosen' && chosenSupplier.value) supplierId = chosenSupplier.value.id
  return {
    payload: payload(),
    decision: decision.value,
    target_product_id: decision.value === 'update_existing' && targetId.value ? targetId.value : null,
    captured_page_id: app.capture?.id ?? null,
    supplier_id: supplierId,
    supplier_contact_id: null,
    create_supplier: supplierChoice.value === 'create',
    tag_names: form.tags.split(',').map((t) => t.trim()).filter(Boolean).slice(0, 20).map((t) => t.slice(0, 60)),
    source: 'extension',
  }
}

async function runCheck(): Promise<void> {
  checking.value = true
  error.value = null
  outcome.value = null
  try {
    check.value = await call({ type: 'import.check', request: request() })
    const dups = check.value.duplicates
    decision.value = check.value.definite_duplicate ? '' : 'create_new'
    targetId.value = dups[0]?.id ?? ''
    supplierChoice.value = check.value.suggested_supplier ? 'suggested' : 'auto'
  } catch (e) {
    error.value = asUiError(e)
  } finally {
    checking.value = false
  }
}

async function findSuppliers(): Promise<void> {
  findingSuppliers.value = true
  try {
    supplierResults.value = await call({ type: 'suppliers.find', q: supplierQuery.value })
  } catch (e) {
    error.value = asUiError(e)
  } finally {
    findingSuppliers.value = false
  }
}

async function submit(): Promise<void> {
  submitting.value = true
  error.value = null
  try {
    if (withCapture.value && !app.capture && can('captures.write')) {
      const saved = await saveCapture()
      if (!saved) {
        error.value = app.captureError
        return
      }
    }
    outcome.value = await call({ type: 'import.submit', request: request() })
  } catch (e) {
    error.value = asUiError(e)
  } finally {
    submitting.value = false
  }
}

const outcomeText: Record<string, string> = {
  imported: 'Imported as a draft product. It is waiting in REA\'s review queue.',
  updated: 'The existing product was refreshed with this capture.',
  skipped: 'The capture was recorded as skipped.',
  needs_decision: 'REA found a definite duplicate and parked the import in the queue for a reviewer to decide.',
}
const canImport = computed(() => !!app.status?.connected && can('products.write'))
const fieldErrors = computed(() => Object.entries(check.value?.errors ?? {}) as [string, string][])

watch(() => x.value?.extractedAt, (v) => { if (v && v !== loadedFrom.value) init() }, { immediate: true })
watch(() => app.selections, (list) => applySelections(list), { deep: true })
</script>

<template>
  <SectionCard v-if="!x" title="Product importer" subtitle="Read the page first to extract product fields.">
    <button type="button" class="btn-primary w-full" :disabled="!app.tab || app.extracting" @click="extract">
      <Spinner v-if="app.extracting" /> Read page
    </button>
    <ErrorNote :error="app.extractError" />
  </SectionCard>

  <template v-else>
    <SectionCard title="Product importer" :subtitle="`Fields from ${x.product.payload.extraction_method}. Overall confidence ${percent(x.product.payload.confidence)}.`">
      <Notice>Imports land in REA's review queue as drafts. Nothing is published, purchased or sent to the supplier.</Notice>
      <Notice v-if="!x.product.detected" kind="warn">This page does not look like a single product page. Check every field before importing.</Notice>
      <div class="space-y-2" data-testid="import-form">
        <div>
          <div class="flex items-center justify-between"><label class="label" for="f-title">Title</label><ConfBadge :value="info('title')?.confidence" :evidence="info('title')?.evidence" /></div>
          <input id="f-title" v-model="form.title" class="input" maxlength="500" @input="touch('title')" />
        </div>
        <div class="grid grid-cols-[1fr_5rem] gap-2">
          <div>
            <div class="flex items-center justify-between"><label class="label" for="f-price">Price</label><ConfBadge :value="info('price')?.confidence" :evidence="info('price')?.evidence" /></div>
            <input id="f-price" v-model="form.price" class="input" maxlength="200" placeholder="No price found" @input="touch('price')" />
          </div>
          <div>
            <label class="label" for="f-currency">Currency</label>
            <input id="f-currency" v-model="form.currency" class="input uppercase" maxlength="3" placeholder="USD" @input="touch('currency')" />
          </div>
        </div>
        <p v-if="form.price && parsedPrice" class="muted -mt-1">Reads as {{ parsedPrice.amount }} {{ form.currency || parsedPrice.currency || '(currency unknown)' }}</p>
        <p v-else-if="form.price" class="-mt-1 text-xs text-amber-700">This price could not be read as a number; REA will keep it as text and warn.</p>
        <div class="grid grid-cols-2 gap-2">
          <div v-for="f in TEXT_FIELDS" :key="f.key">
            <div class="flex items-center justify-between"><label class="label" :for="`f-${f.key}`">{{ f.label }}</label><ConfBadge :value="info(f.key)?.confidence" :evidence="info(f.key)?.evidence" /></div>
            <input :id="`f-${f.key}`" v-model="form[f.key as TextKey]" class="input" :maxlength="f.max" @input="touch(f.key)" />
          </div>
        </div>
        <div>
          <div class="flex items-center justify-between"><label class="label" for="f-desc">Description</label><ConfBadge :value="info('description')?.confidence" :evidence="info('description')?.evidence" /></div>
          <textarea id="f-desc" v-model="form.description" class="input min-h-20" maxlength="50000" @input="touch('description')" />
        </div>
        <div class="grid grid-cols-2 gap-2">
          <div>
            <div class="flex items-center justify-between"><label class="label" for="f-sup">Supplier name</label><ConfBadge :value="info('supplier_name')?.confidence" :evidence="info('supplier_name')?.evidence" /></div>
            <input id="f-sup" v-model="form.supplier_name" class="input" maxlength="200" @input="touch('supplier_name')" />
          </div>
          <div>
            <label class="label" for="f-supurl">Supplier website</label>
            <input id="f-supurl" v-model="form.supplier_url" class="input" maxlength="2048" type="url" @input="touch('supplier_url')" />
          </div>
        </div>
        <div>
          <div class="flex items-center justify-between"><p class="label">Images ({{ form.images.filter((i) => i.keep).length }})</p><ConfBadge :value="info('images')?.confidence" :evidence="info('images')?.evidence" /></div>
          <p v-if="!form.images.length" class="muted">No images found. Use the element picker to choose one.</p>
          <ul class="grid grid-cols-4 gap-1.5">
            <li v-for="(img, i) in form.images.slice(0, 16)" :key="img.url" class="relative">
              <label class="block cursor-pointer">
                <img :src="img.url" :alt="img.alt || `Image ${i + 1}`" class="aspect-square w-full rounded border object-cover" :class="img.keep ? 'border-brand-600' : 'border-slate-200 opacity-40'" referrerpolicy="no-referrer" loading="lazy" />
                <input v-model="img.keep" type="checkbox" class="absolute top-1 left-1" :aria-label="`Keep image ${i + 1}`" @change="touch('images')" />
              </label>
            </li>
          </ul>
        </div>
        <div>
          <div class="flex items-center justify-between"><p class="label">Specifications ({{ form.specs.length }})</p><ConfBadge :value="info('specifications')?.confidence" :evidence="info('specifications')?.evidence" /></div>
          <div v-for="(s, i) in form.specs" :key="i" class="mb-1 grid grid-cols-[1fr_1.4fr_auto] gap-1">
            <input v-model="s.key" class="input py-1 text-xs" maxlength="120" :aria-label="`Specification ${i + 1} name`" @input="touch('specifications')" />
            <input v-model="s.value" class="input py-1 text-xs" maxlength="2000" :aria-label="`Specification ${i + 1} value`" @input="touch('specifications')" />
            <button type="button" class="text-slate-400 hover:text-red-600" :aria-label="`Remove specification ${i + 1}`" @click="form.specs.splice(i, 1); touch('specifications')"><Trash2 class="size-3.5" /></button>
          </div>
          <button type="button" class="btn-secondary px-2 py-1 text-xs" @click="form.specs.push({ key: '', value: '' })"><Plus class="size-3.5" aria-hidden="true" /> Add row</button>
        </div>
        <p v-if="base && base.variants.length" class="muted">{{ base.variants.length }} variant(s) will be imported: {{ base.variants.slice(0, 4).map((v) => v.title || v.sku).join(', ') }}{{ base.variants.length > 4 ? '…' : '' }}</p>
        <div>
          <label class="label" for="f-tags">Tags (comma separated)</label>
          <input id="f-tags" v-model="form.tags" class="input" maxlength="600" placeholder="sourcing, kitchen" />
        </div>
      </div>
      <Notice v-if="!canImport" kind="warn">{{ app.status?.connected ? 'Your REA role cannot import products (products.write).' : 'Pair the extension with REA to import products.' }}</Notice>
      <button type="button" class="btn-secondary w-full" :disabled="!canImport || checking || !form.title.trim()" data-testid="import-check" @click="runCheck">
        <Spinner v-if="checking" /><Search v-else class="size-4" aria-hidden="true" /> Check with REA
      </button>
      <ErrorNote :error="error" />
    </SectionCard>

    <SectionCard v-if="check" title="Check result" subtitle="REA normalized the fields and looked for duplicates. Nothing was stored yet." data-testid="check-result">
      <div v-if="fieldErrors.length" role="alert" class="rounded-md border border-red-200 bg-red-50 p-2 text-xs text-red-800">
        <p class="font-medium">Fix these fields before importing:</p>
        <ul class="list-disc pl-4"><li v-for="[f, m] in fieldErrors" :key="f"><span class="font-medium">{{ f }}</span> {{ m }}</li></ul>
      </div>
      <dl class="grid grid-cols-[6.5rem_1fr] gap-x-2 gap-y-1 text-[13px]">
        <dt class="text-slate-500">Title</dt><dd>{{ check.normalized.title }}</dd>
        <dt class="text-slate-500">Price</dt><dd>{{ check.normalized.price ?? 'none' }} {{ check.normalized.price_currency ?? '' }}</dd>
        <dt class="text-slate-500">Availability</dt><dd>{{ check.normalized.availability }}</dd>
        <dt class="text-slate-500">Source</dt><dd class="break-all">{{ check.normalized.source_domain }}</dd>
        <dt class="text-slate-500">Images</dt><dd>{{ check.normalized.images.length }}</dd>
        <dt class="text-slate-500">Variants</dt><dd>{{ check.normalized.variants.length }}</dd>
      </dl>
      <Notice v-if="check.warnings.length" kind="warn">
        <ul class="list-disc pl-4 text-xs"><li v-for="w in check.warnings" :key="w">{{ w }}</li></ul>
      </Notice>
      <div v-if="check.duplicates.length" class="space-y-1">
        <p class="text-xs font-medium text-slate-700">{{ check.definite_duplicate ? 'Definite duplicate found' : 'Possible duplicates' }}</p>
        <ul class="divide-y divide-slate-100 rounded-md border border-slate-200">
          <li v-for="d in check.duplicates" :key="d.id" class="p-2 text-xs">
            <div class="flex items-center justify-between gap-2">
              <ExtLink :href="link(`/products/${d.id}`)">{{ d.label }}</ExtLink>
              <span class="chip border-slate-200 bg-slate-50 text-slate-600">{{ percent(d.score) }}</span>
            </div>
            <p class="text-slate-500">{{ d.reasons.join(', ') }}</p>
          </li>
        </ul>
      </div>
      <p v-else class="muted">No duplicates found in REA.</p>

      <fieldset class="space-y-1">
        <legend class="label">Decision</legend>
        <label class="flex gap-2 text-[13px]"><input v-model="decision" type="radio" value="create_new" /> Create a new draft product</label>
        <label v-if="check.duplicates.length" class="flex gap-2 text-[13px]"><input v-model="decision" type="radio" value="update_existing" /> Update an existing product</label>
        <select v-if="decision === 'update_existing'" v-model="targetId" class="input ml-5 w-auto" aria-label="Product to update">
          <option v-for="d in check.duplicates" :key="d.id" :value="d.id">{{ d.label }}</option>
        </select>
        <label v-if="check.duplicates.length" class="flex gap-2 text-[13px]"><input v-model="decision" type="radio" value="" /> Let REA decide (definite duplicates wait for a reviewer)</label>
        <label class="flex gap-2 text-[13px]"><input v-model="decision" type="radio" value="skip" /> Skip this product</label>
      </fieldset>

      <fieldset class="space-y-1">
        <legend class="label">Supplier</legend>
        <label v-if="suggested" class="flex gap-2 text-[13px]"><input v-model="supplierChoice" type="radio" value="suggested" /> Link to {{ suggested.label }} ({{ suggested.reasons.join(', ') }})</label>
        <label class="flex gap-2 text-[13px]"><input v-model="supplierChoice" type="radio" value="auto" /> Automatic (REA links a supplier whose domain matches)</label>
        <label class="flex gap-2 text-[13px]"><input v-model="supplierChoice" type="radio" value="chosen" /> Choose an existing supplier</label>
        <div v-if="supplierChoice === 'chosen'" class="ml-5 space-y-1">
          <div class="flex gap-1">
            <input v-model="supplierQuery" class="input" placeholder="Supplier name" aria-label="Search suppliers" @keydown.enter.prevent="findSuppliers" />
            <button type="button" class="btn-secondary" :disabled="findingSuppliers || supplierQuery.trim().length < 2" @click="findSuppliers"><Spinner v-if="findingSuppliers" />Find</button>
          </div>
          <label v-for="s in supplierResults" :key="s.id" class="flex gap-2 text-xs">
            <input type="radio" name="chosen-supplier" :checked="chosenSupplier?.id === s.id" @change="chosenSupplier = s" /> {{ s.name }} <span class="text-slate-500">{{ s.domain }}</span>
          </label>
        </div>
        <label v-if="form.supplier_name && can('crm.write')" class="flex gap-2 text-[13px]"><input v-model="supplierChoice" type="radio" value="create" /> Create supplier "{{ form.supplier_name }}" if none matches</label>
      </fieldset>

      <label v-if="can('captures.write')" class="flex gap-2 text-[13px]">
        <input v-model="withCapture" type="checkbox" :disabled="!!app.capture" />
        {{ app.capture ? 'The saved capture is attached as evidence' : 'Save the captured page as evidence' }}
      </label>
      <button
        type="button"
        class="btn-primary w-full"
        :disabled="submitting || fieldErrors.length > 0 || (decision === 'update_existing' && !targetId) || (supplierChoice === 'chosen' && !chosenSupplier)"
        data-testid="import-submit"
        @click="submit"
      >
        <Spinner v-if="submitting" /><PackagePlus v-else class="size-4" aria-hidden="true" /> Send to review queue
      </button>
      <Notice v-if="outcome" kind="success">
        <p data-testid="import-outcome">{{ outcomeText[outcome.outcome.status] ?? outcome.outcome.status }}</p>
        <ul v-if="outcome.outcome.warnings.length" class="list-disc pl-4 text-xs"><li v-for="w in outcome.outcome.warnings" :key="w">{{ w }}</li></ul>
        <div class="flex flex-wrap gap-3">
          <ExtLink :href="link(`/imports/${outcome.outcome.import.id}`)">View import</ExtLink>
          <ExtLink :href="link('/imports')">Review queue</ExtLink>
          <ExtLink v-if="outcome.outcome.import.product_id" :href="link(`/products/${outcome.outcome.import.product_id}`)">Draft product</ExtLink>
        </div>
      </Notice>
    </SectionCard>
  </template>
</template>
