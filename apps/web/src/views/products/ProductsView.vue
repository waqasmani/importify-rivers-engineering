<script setup lang="ts">
// Product catalog with filters, facets, sorting, bulk actions, export, CSV import and manual creation.
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { refDebounced } from '@vueuse/core'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { ImageOff, Lock, Plus, RefreshCw, Upload } from 'lucide-vue-next'
import { api, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import DataTable, { type Column } from '@/components/ui/DataTable.vue'
import UiPagination from '@/components/ui/UiPagination.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiButton from '@/components/ui/UiButton.vue'
import EntityPicker from '@/components/record/EntityPicker.vue'
import ExportButton from '@/components/record/ExportButton.vue'
import CsvImportDialog from '@/components/record/CsvImportDialog.vue'
import ProductFormModal from '@/components/products/ProductFormModal.vue'
import BulkProductActions from '@/components/products/BulkProductActions.vue'
import RepriceDialog from '@/components/products/RepriceDialog.vue'
import { useListQuery } from '@/composables/useListQuery'
import { useAuth } from '@/stores/auth'
import { useSettings } from '@/stores/settings'
import { formatMoney, relativeTime, formatDateTime, titleCase } from '@/lib/format'
import { AVAILABILITIES, PRODUCT_SORTS, PRODUCT_STATUSES, REVIEW_LABELS, REVIEW_STATUSES } from '@/components/products/product-utils'

type Row = Schemas['ProductView']
const FILTERS = ['q', 'status', 'review_status', 'supplier_id', 'source_domain', 'category', 'availability', 'tag_id']

const auth = useAuth()
const router = useRouter()
const qc = useQueryClient()
const { label } = useSettings()
const list = useListQuery<Row>('products', '/products', FILTERS)

const search = ref(list.filters.q ?? '')
const searchDebounced = refDebounced(search, 300)
watch(searchDebounced, (v) => {
  if (v.trim() !== (list.filters.q ?? '')) list.set('q', v.trim())
})
watch(
  () => list.filters.q,
  (v) => {
    if ((v ?? '') !== search.value.trim()) search.value = v ?? ''
  },
)

const facets = useQuery({ queryKey: ['products', 'facets'], queryFn: ({ signal }) => api.get<Schemas['FacetsDoc']>('/products/facets', undefined, signal), staleTime: 30_000 })
const tags = useQuery({ queryKey: ['tags'], queryFn: ({ signal }) => api.get<Schemas['ListTagsRowItems']>('/tags', undefined, signal), staleTime: 60_000 })
const showAllDomains = ref(false)
const showAllCategories = ref(false)
const domains = computed(() => (facets.data.value?.domains ?? []).slice(0, showAllDomains.value ? 100 : 10))
const categories = computed(() => (facets.data.value?.categories ?? []).slice(0, showAllCategories.value ? 200 : 10))

function toggle(key: string, v: string) {
  list.set(key, list.filters[key] === v ? '' : v)
}

const columns = computed<Column[]>(() => [
  { key: 'image', label: 'Image', class: 'w-16' },
  { key: 'title', label: label('product'), sortKey: 'title' },
  { key: 'price', label: 'Price', sortKey: 'price', class: 'hidden sm:table-cell text-right' },
  { key: 'margin', label: 'Margin', sortKey: 'margin', class: 'hidden md:table-cell text-right' },
  { key: 'review_status', label: 'Review', class: 'hidden sm:table-cell' },
  { key: 'supplier_name', label: label('supplier'), class: 'hidden lg:table-cell' },
  { key: 'availability', label: 'Availability', class: 'hidden xl:table-cell' },
  { key: 'created', label: 'Added', sortKey: 'created', class: 'hidden 2xl:table-cell' },
  { key: 'checked', label: 'Last checked', sortKey: 'checked', class: 'hidden xl:table-cell' },
])

// The server accepts title, price, -price, -margin, -created and checked. Clicking a column cycles
// through the directions it supports and then back to the default (recently updated).
function onSort(v: string) {
  const allowed = PRODUCT_SORTS as readonly string[]
  if (allowed.includes(v) && list.filters.sort !== v) return list.set('sort', v)
  const flipped = v.startsWith('-') ? v.slice(1) : `-${v}`
  if (allowed.includes(flipped) && list.filters.sort !== flipped) return list.set('sort', flipped)
  list.set('sort', '')
}
const tableSort = computed(() => list.filters.sort ?? '')

const selected = ref<string[]>([])
watch(() => list.items.value, (rows) => (selected.value = selected.value.filter((id) => rows.some((r) => r.id === id))))
const canBulk = computed(() => auth.can('products.write') || auth.can('products.delete') || auth.can('imports.review'))

function afterBulk() {
  selected.value = []
  void qc.invalidateQueries({ queryKey: ['products'] })
  void qc.invalidateQueries({ queryKey: ['dashboard'] })
}

const showFilters = ref(false)
const activeFilterCount = computed(() => ['status', 'review_status', 'supplier_id', 'availability', 'tag_id', 'sort'].filter((k) => !!list.filters[k]).length)
const creating = ref(false)
const importing = ref(false)
const filtered = computed(() => FILTERS.some((k) => !!list.filters[k]))
const exportQuery = computed(() => Object.fromEntries(Object.entries(list.filters).filter(([, v]) => !!v)) as Record<string, string>)
// Reprice everything the current filters match (the server caps one run at 1000 products).
const repricingAll = ref(false)
const repriceFilter = computed<Schemas['RepriceFilter']>(() => {
  const f = list.filters
  return {
    q: f.q ?? '', status: f.status ?? '', review_status: f.review_status ?? '', source_domain: f.source_domain ?? '', category: f.category ?? '',
    availability: f.availability ?? '', supplier_id: f.supplier_id || null, tag_id: f.tag_id || null,
  }
})

function onCreated(p: Schemas['ProductDetail']) {
  void qc.invalidateQueries({ queryKey: ['products'] })
  void router.push(`/products/${p.id}`)
}
function onImported() {
  void qc.invalidateQueries({ queryKey: ['products'] })
  void qc.invalidateQueries({ queryKey: ['imports'] })
}
</script>

<template>
  <div>
    <PageHeader :title="label('product', true)" subtitle="Products stored in REA. Approval changes REA's own review status only; nothing is published to a store.">
      <template #actions>
        <ExportButton v-if="auth.can('data.export')" path="/products/export" :query="exportQuery" name="products" />
        <UiButton v-if="auth.can('products.write')" :disabled="list.total.value === 0" @click="repricingAll = true"><RefreshCw class="h-4 w-4" aria-hidden="true" /> Reprice{{ filtered ? ' matching' : ' all' }}</UiButton>
        <UiButton v-if="auth.can('data.import') && auth.can('products.write')" @click="importing = true"><Upload class="h-4 w-4" /> Import CSV</UiButton>
        <RouterLink v-if="auth.can('products.write')" to="/imports/new" class="rounded-md border border-slate-300 bg-white px-3.5 py-2 text-sm font-medium text-slate-700 shadow-xs hover:bg-slate-50">Import from page</RouterLink>
        <UiButton v-if="auth.can('products.write')" variant="primary" @click="creating = true"><Plus class="h-4 w-4" /> New {{ label('product').toLowerCase() }}</UiButton>
      </template>
    </PageHeader>

    <form class="mb-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-4" role="search" :aria-label="`Filter ${label('product', true).toLowerCase()}`" @submit.prevent="list.set('q', search.trim())">
      <div class="sm:col-span-2">
        <label for="p-q" class="label">Search</label>
        <div class="flex gap-2">
          <input id="p-q" v-model="search" type="search" class="input" placeholder="Title, brand, SKU or description" />
          <button type="button" class="shrink-0 rounded-md border border-slate-300 bg-white px-3 text-sm text-slate-700 sm:hidden" :aria-expanded="showFilters" @click="showFilters = !showFilters">
            Filters<span v-if="activeFilterCount"> ({{ activeFilterCount }})</span>
          </button>
        </div>
      </div>
      <div :class="showFilters ? '' : 'hidden sm:block'">
        <label for="p-review" class="label">Review status</label>
        <select id="p-review" class="input" :value="list.filters.review_status" @change="list.set('review_status', ($event.target as HTMLSelectElement).value)">
          <option value="">Any</option>
          <option v-for="s in REVIEW_STATUSES" :key="s" :value="s">{{ REVIEW_LABELS[s] }}</option>
        </select>
      </div>
      <div :class="showFilters ? '' : 'hidden sm:block'">
        <label for="p-status" class="label">Status</label>
        <select id="p-status" class="input" :value="list.filters.status" @change="list.set('status', ($event.target as HTMLSelectElement).value)">
          <option value="">Any</option>
          <option v-for="s in PRODUCT_STATUSES" :key="s" :value="s">{{ titleCase(s) }}</option>
        </select>
      </div>
      <div :class="showFilters ? '' : 'hidden sm:block'">
        <label for="p-avail" class="label">Availability</label>
        <select id="p-avail" class="input" :value="list.filters.availability" @change="list.set('availability', ($event.target as HTMLSelectElement).value)">
          <option value="">Any</option>
          <option v-for="a in AVAILABILITIES" :key="a" :value="a">{{ titleCase(a) }}</option>
        </select>
      </div>
      <div v-if="auth.can('crm.read')" :class="showFilters ? '' : 'hidden sm:block'">
        <label for="p-supplier" class="label">{{ label('supplier') }}</label>
        <EntityPicker
          :model-value="list.filters.supplier_id || null"
          type="supplier"
          input-id="p-supplier"
          :placeholder="`Any ${label('supplier').toLowerCase()}`"
          @update:model-value="(v: string | null) => list.set('supplier_id', v ?? '')"
        />
      </div>
      <div :class="showFilters ? '' : 'hidden sm:block'">
        <label for="p-tag" class="label">Tag</label>
        <select id="p-tag" class="input" :value="list.filters.tag_id" @change="list.set('tag_id', ($event.target as HTMLSelectElement).value)">
          <option value="">Any tag</option>
          <option v-for="t in tags.data.value?.items ?? []" :key="t.id" :value="t.id">{{ t.name }}</option>
        </select>
      </div>
      <div :class="showFilters ? '' : 'hidden sm:block'">
        <label for="p-sort" class="label">Sort by</label>
        <select id="p-sort" class="input" :value="list.filters.sort" @change="list.set('sort', ($event.target as HTMLSelectElement).value)">
          <option value="">Recently updated</option>
          <option value="title">Title A–Z</option>
          <option value="price">Price, low to high</option>
          <option value="-price">Price, high to low</option>
          <option value="-margin">Highest margin</option>
          <option value="-created">Newest first</option>
          <option value="checked">Least recently checked</option>
        </select>
      </div>
    </form>

    <div v-if="domains.length || categories.length || list.filters.source_domain || list.filters.category" class="mb-4 space-y-2" data-testid="facets">
      <div v-if="domains.length || list.filters.source_domain" class="flex flex-wrap items-center gap-1.5" role="group" aria-label="Source domain">
        <span class="mr-1 text-xs font-semibold tracking-wide text-slate-500 uppercase">Source</span>
        <button
          v-for="d in domains"
          :key="d.source_domain"
          type="button"
          class="rounded-full border px-2.5 py-0.5 text-xs"
          :class="list.filters.source_domain === d.source_domain ? 'border-brand-600 bg-brand-600 text-white' : 'border-slate-300 bg-white text-slate-700 hover:border-brand-300'"
          :aria-pressed="list.filters.source_domain === d.source_domain"
          @click="toggle('source_domain', d.source_domain)"
        >
          {{ d.source_domain }} <span class="opacity-70">{{ d.product_count }}</span>
        </button>
        <button v-if="list.filters.source_domain && !domains.some((d) => d.source_domain === list.filters.source_domain)" type="button" class="rounded-full border border-brand-600 bg-brand-600 px-2.5 py-0.5 text-xs text-white" aria-pressed="true" @click="toggle('source_domain', list.filters.source_domain!)">{{ list.filters.source_domain }}</button>
        <button v-if="(facets.data.value?.domains.length ?? 0) > 10" type="button" class="text-xs text-brand-700 hover:underline" @click="showAllDomains = !showAllDomains">{{ showAllDomains ? 'Show fewer' : 'Show all' }}</button>
      </div>
      <div v-if="categories.length || list.filters.category" class="flex flex-wrap items-center gap-1.5" role="group" aria-label="Category">
        <span class="mr-1 text-xs font-semibold tracking-wide text-slate-500 uppercase">Category</span>
        <button
          v-for="c in categories"
          :key="c.category"
          type="button"
          class="rounded-full border px-2.5 py-0.5 text-xs"
          :class="list.filters.category === c.category ? 'border-brand-600 bg-brand-600 text-white' : 'border-slate-300 bg-white text-slate-700 hover:border-brand-300'"
          :aria-pressed="list.filters.category === c.category"
          @click="toggle('category', c.category)"
        >
          {{ c.category }} <span class="opacity-70">{{ c.product_count }}</span>
        </button>
        <button v-if="list.filters.category && !categories.some((c) => c.category === list.filters.category)" type="button" class="rounded-full border border-brand-600 bg-brand-600 px-2.5 py-0.5 text-xs text-white" aria-pressed="true" @click="toggle('category', list.filters.category!)">{{ list.filters.category }}</button>
        <button v-if="(facets.data.value?.categories.length ?? 0) > 10" type="button" class="text-xs text-brand-700 hover:underline" @click="showAllCategories = !showAllCategories">{{ showAllCategories ? 'Show fewer' : 'Show all' }}</button>
      </div>
    </div>

    <div class="mb-2 flex flex-wrap items-center justify-between gap-2">
      <p class="text-sm text-slate-500" aria-live="polite">{{ list.total.value }} {{ list.total.value === 1 ? label('product').toLowerCase() : label('product', true).toLowerCase() }}</p>
      <button v-if="filtered" type="button" class="text-sm text-brand-700 hover:underline" @click="search = ''; $router.replace({ query: list.filters.sort ? { sort: list.filters.sort } : {} })">Clear filters</button>
    </div>

    <BulkProductActions v-if="canBulk && selected.length" :ids="selected" class="mb-3" @done="afterBulk" />

    <DataTable
      v-model:selected="selected"
      :columns="columns"
      :rows="list.items.value"
      :loading="list.loading.value"
      :error="list.error.value"
      :selectable="canBulk"
      :sort="tableSort"
      :empty-title="filtered ? `No ${label('product', true).toLowerCase()} match these filters` : `No ${label('product', true).toLowerCase()} yet`"
      empty-message="Import products from a web page with the browser extension, upload a CSV file, or add one by hand."
      @sort="onSort"
      @retry="list.refetch()"
    >
      <template #cell-image="{ row }">
        <img v-if="row.primary_image_url" :src="row.primary_image_url" alt="" loading="lazy" referrerpolicy="no-referrer" class="h-10 w-10 rounded border border-slate-200 bg-white object-cover" />
        <span v-else class="flex h-10 w-10 items-center justify-center rounded border border-dashed border-slate-200 text-slate-300"><ImageOff class="h-4 w-4" aria-label="No image" /></span>
      </template>
      <template #cell-title="{ row }">
        <div class="max-w-xs min-w-40">
          <RouterLink :to="`/products/${row.id}`" class="line-clamp-2 font-medium text-brand-700 hover:underline">{{ row.title }}</RouterLink>
          <p class="truncate text-xs text-slate-500">{{ [row.brand, row.sku, row.source_domain].filter(Boolean).join(' · ') || '—' }}</p>
          <p class="mt-1 flex flex-wrap items-center gap-2 text-xs sm:hidden">
            <span class="font-medium text-slate-800">{{ formatMoney(row.price, row.price_currency) }}</span>
            <UiBadge :value="row.review_status">{{ REVIEW_LABELS[row.review_status] ?? row.review_status }}</UiBadge>
          </p>
          <div v-if="row.tags.length" class="mt-1 flex flex-wrap gap-1">
            <UiBadge v-for="t in row.tags.slice(0, 3)" :key="t.id" :color="t.color || undefined">{{ t.name }}</UiBadge>
          </div>
        </div>
      </template>
      <template #cell-price="{ row }">
        <span class="inline-flex items-center gap-1 whitespace-nowrap">
          <Lock v-if="row.price_locked" class="h-3.5 w-3.5 text-sky-600" aria-label="Locked manual price" />
          {{ formatMoney(row.price, row.price_currency) }}
        </span>
        <span v-if="row.pricing_rule_name && !row.price_locked" class="block truncate text-xs text-slate-500" :title="`Pricing rule: ${row.pricing_rule_name}`">{{ row.pricing_rule_name }}</span>
        <span v-if="row.variant_count" class="block text-xs text-slate-500">{{ row.variant_count }} variant{{ row.variant_count === 1 ? '' : 's' }}</span>
      </template>
      <template #cell-margin="{ row }">
        <span v-if="row.margin_percent !== null" class="whitespace-nowrap">{{ Number(row.margin_percent).toFixed(1) }}%</span>
        <span v-else class="text-slate-400">—</span>
      </template>
      <template #cell-review_status="{ row }">
        <div class="flex flex-col items-start gap-1">
          <UiBadge :value="row.review_status">{{ REVIEW_LABELS[row.review_status] ?? row.review_status }}</UiBadge>
          <span class="text-xs text-slate-500">{{ titleCase(row.status) }}</span>
        </div>
      </template>
      <template #cell-supplier_name="{ row }">
        <RouterLink v-if="row.supplier_id && auth.can('crm.read')" :to="`/suppliers/${row.supplier_id}`" class="hover:underline">{{ row.supplier_name ?? 'Linked' }}</RouterLink>
        <span v-else class="text-slate-400">—</span>
      </template>
      <template #cell-availability="{ row }"><UiBadge :value="row.availability" /></template>
      <template #cell-created="{ row }"><time :datetime="row.created_at" :title="formatDateTime(row.created_at)" class="whitespace-nowrap">{{ relativeTime(row.created_at) }}</time></template>
      <template #cell-checked="{ row }"><time :datetime="row.last_checked_at" :title="formatDateTime(row.last_checked_at)" class="whitespace-nowrap">{{ relativeTime(row.last_checked_at) }}</time></template>
    </DataTable>
    <UiPagination :total="list.total.value" :limit="list.limit.value" :offset="list.offset.value" @update:offset="(o) => list.set('offset', String(o))" />

    <ProductFormModal :open="creating" @close="creating = false" @saved="onCreated" />
    <RepriceDialog :open="repricingAll" :filter="repriceFilter" :noun="label('product', true).toLowerCase()" @close="repricingAll = false" @applied="afterBulk" />
    <CsvImportDialog :open="importing" entity-type="product" :label="label('product', true)" @close="importing = false" @imported="onImported" />
  </div>
</template>
