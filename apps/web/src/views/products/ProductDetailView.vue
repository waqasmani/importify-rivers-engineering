<script setup lang="ts">
// One product: gallery, pricing, identifiers, specifications, variants, suppliers, provenance,
// review, duplicates and the shared record panels.
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { ExternalLink, Pencil, Trash2 } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import StateViews from '@/components/ui/StateViews.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiButton from '@/components/ui/UiButton.vue'
import TagEditor from '@/components/record/TagEditor.vue'
import CustomFieldsEditor from '@/components/record/CustomFieldsEditor.vue'
import RecordPanels from '@/components/record/RecordPanels.vue'
import ImageGallery from '@/components/products/ImageGallery.vue'
import VariantsEditor from '@/components/products/VariantsEditor.vue'
import SupplierLinks from '@/components/products/SupplierLinks.vue'
import ReviewPanel from '@/components/products/ReviewPanel.vue'
import ProvenancePanel from '@/components/products/ProvenancePanel.vue'
import ProductDuplicatesPanel from '@/components/products/ProductDuplicatesPanel.vue'
import ProductFormModal from '@/components/products/ProductFormModal.vue'
import { useAuth } from '@/stores/auth'
import { useSettings } from '@/stores/settings'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { formatMoney, titleCase } from '@/lib/format'
import { asRecord, safeHref, stringMap, validGtin, REVIEW_LABELS } from '@/components/products/product-utils'

type Detail = Schemas['ProductDetail']

const props = defineProps<{ id: string }>()
const auth = useAuth()
const router = useRouter()
const qc = useQueryClient()
const toast = useToast()
const { confirm } = useConfirm()
const { label } = useSettings()

const key = computed(() => ['product', props.id])
const q = useQuery({ queryKey: key, queryFn: ({ signal }) => api.get<Detail>(`/products/${props.id}`, undefined, signal) })
const p = computed(() => q.data.value)
const canWrite = computed(() => auth.can('products.write'))
const editing = ref(false)

const specs = computed(() => Object.entries(stringMap(p.value?.specifications)))
const identifiers = computed(() => Object.entries(stringMap(p.value?.identifiers)).filter(([k]) => k !== 'gtin_unverified'))
const unverifiedGtin = computed(() => stringMap(p.value?.identifiers).gtin_unverified ?? '')
const gtinValid = computed(() => validGtin(p.value?.gtin))
const marginNote = computed(() => {
  const x = p.value
  if (!x) return ''
  if (x.margin_percent !== null) return ''
  if (x.price && x.cost && x.price_currency !== x.cost_currency) return 'Not calculated: price and cost use different currencies.'
  if (!x.price || !x.cost) return 'Needs both a price and a cost.'
  return ''
})
const customFields = computed(() => asRecord(p.value?.custom_fields))

function refresh() {
  void qc.invalidateQueries({ queryKey: key.value })
  void qc.invalidateQueries({ queryKey: ['products'] })
  void qc.invalidateQueries({ queryKey: ['activity', 'product', props.id] })
}
function onReviewed() {
  refresh()
  void qc.invalidateQueries({ queryKey: ['notes', 'product', props.id] })
  void qc.invalidateQueries({ queryKey: ['dashboard'] })
}
function onSaved(d: Detail) {
  qc.setQueryData(key.value, d)
  refresh()
  void qc.invalidateQueries({ queryKey: ['product-suppliers', props.id] })
}
function onSuppliersChanged() {
  refresh()
}
function onMerged(d: Detail) {
  qc.setQueryData(key.value, d)
  refresh()
  void qc.invalidateQueries({ queryKey: ['product-suppliers', props.id] })
}
function onTags(tags: Schemas['TagRef'][]) {
  if (p.value) qc.setQueryData(key.value, { ...p.value, tags })
  void qc.invalidateQueries({ queryKey: ['products'] })
}

async function remove() {
  if (!p.value) return
  const ok = await confirm({
    title: `Delete ${label('product').toLowerCase()}`,
    message: `Delete “${p.value.title}” with its variants, images, supplier links, notes, files and tasks? This cannot be undone.`,
    confirmLabel: 'Delete',
    danger: true,
  })
  if (!ok) return
  try {
    await api.del(`/products/${props.id}`)
    toast.success(`${label('product')} deleted.`)
    void qc.invalidateQueries({ queryKey: ['products'] })
    qc.removeQueries({ queryKey: key.value })
    await router.replace('/products')
  } catch (e) {
    toast.error(errorMessage(e))
  }
}
</script>

<template>
  <div>
    <StateViews v-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <template v-else-if="p">
      <PageHeader :title="p.title" back="/products">
        <template #meta>
          <div class="mt-2 flex flex-wrap items-center gap-2 text-sm text-slate-600" data-testid="product-meta">
            <UiBadge :value="p.review_status">{{ REVIEW_LABELS[p.review_status] ?? p.review_status }}</UiBadge>
            <UiBadge :value="p.status">{{ titleCase(p.status) }}</UiBadge>
            <UiBadge :value="p.availability" />
            <a v-if="p.source_domain && safeHref(p.source_url)" :href="safeHref(p.source_url)" target="_blank" rel="noopener noreferrer nofollow" class="inline-flex items-center gap-1 text-brand-700 hover:underline">
              {{ p.source_domain }} <ExternalLink class="h-3.5 w-3.5" aria-hidden="true" /><span class="sr-only">(opens the source page)</span>
            </a>
            <span v-else-if="p.source_domain">{{ p.source_domain }}</span>
            <span v-if="p.supplier_name">·
              <RouterLink v-if="p.supplier_id && auth.can('crm.read')" :to="`/suppliers/${p.supplier_id}`" class="hover:underline">{{ p.supplier_name }}</RouterLink>
              <template v-else>{{ p.supplier_name }}</template>
            </span>
          </div>
          <p v-if="p.original_title && p.original_title !== p.title" class="mt-1 text-xs text-slate-500">Title on the source page: {{ p.original_title }}</p>
        </template>
        <template #actions>
          <UiButton v-if="canWrite" @click="editing = true"><Pencil class="h-4 w-4" /> Edit</UiButton>
          <UiButton v-if="auth.can('products.delete')" variant="danger" @click="remove"><Trash2 class="h-4 w-4" /> Delete</UiButton>
        </template>
      </PageHeader>

      <div class="grid gap-6 lg:grid-cols-3">
        <div class="min-w-0 space-y-6 lg:col-span-2">
          <div class="grid gap-6 md:grid-cols-2">
            <ImageGallery :product-id="p.id" :images="p.images" :title="p.title" :can-edit="canWrite" @changed="refresh" />

            <section class="card p-4" aria-labelledby="pricing-heading" data-testid="pricing">
              <h2 id="pricing-heading" class="text-sm font-semibold text-slate-800">Pricing</h2>
              <dl class="mt-3 grid grid-cols-2 gap-3 text-sm">
                <div>
                  <dt class="text-xs font-medium text-slate-500">Price</dt>
                  <dd class="text-lg font-semibold text-slate-900">{{ formatMoney(p.price, p.price_currency) }}</dd>
                </div>
                <div>
                  <dt class="text-xs font-medium text-slate-500">Cost</dt>
                  <dd class="text-lg font-semibold text-slate-900">{{ formatMoney(p.cost, p.cost_currency) }}</dd>
                </div>
                <div class="col-span-2">
                  <dt class="text-xs font-medium text-slate-500">Margin</dt>
                  <dd v-if="p.margin_percent !== null" class="text-slate-900">{{ Number(p.margin_percent).toFixed(2) }}% · {{ formatMoney(p.margin_amount, p.price_currency) }}</dd>
                  <dd v-else class="text-slate-500">{{ marginNote || '—' }}</dd>
                </div>
              </dl>

              <h3 class="mt-5 text-sm font-semibold text-slate-800">Identifiers</h3>
              <dl class="mt-2 grid grid-cols-2 gap-x-3 gap-y-2 text-sm" data-testid="identifiers">
                <div><dt class="text-xs font-medium text-slate-500">Brand</dt><dd>{{ p.brand || '—' }}</dd></div>
                <div><dt class="text-xs font-medium text-slate-500">Model</dt><dd>{{ p.model || '—' }}</dd></div>
                <div><dt class="text-xs font-medium text-slate-500">SKU</dt><dd class="break-all">{{ p.sku || '—' }}</dd></div>
                <div><dt class="text-xs font-medium text-slate-500">MPN</dt><dd class="break-all">{{ p.mpn || '—' }}</dd></div>
                <div class="col-span-2">
                  <dt class="text-xs font-medium text-slate-500">GTIN</dt>
                  <dd v-if="p.gtin">
                    <span class="font-mono">{{ p.gtin }}</span>
                    <span :class="gtinValid ? 'text-green-700' : 'text-red-700'" class="ml-2 text-xs" data-testid="gtin-hint">{{ gtinValid ? `Valid GTIN-${p.gtin.length} check digit` : 'Check digit does not match' }}</span>
                  </dd>
                  <dd v-else class="text-slate-500">—</dd>
                  <dd v-if="unverifiedGtin" class="mt-0.5 text-xs text-amber-700">Unverified barcode from the source: <span class="font-mono">{{ unverifiedGtin }}</span> (failed its check digit).</dd>
                </div>
                <div v-for="[k, v] in identifiers" :key="k"><dt class="text-xs font-medium text-slate-500">{{ titleCase(k) }}</dt><dd class="break-all">{{ v }}</dd></div>
                <div class="col-span-2"><dt class="text-xs font-medium text-slate-500">Category</dt><dd>{{ p.category || '—' }}</dd></div>
              </dl>
            </section>
          </div>

          <section v-if="p.description" class="card p-4" aria-labelledby="desc-heading">
            <h2 id="desc-heading" class="text-sm font-semibold text-slate-800">Description</h2>
            <p class="mt-2 text-sm whitespace-pre-wrap text-slate-700">{{ p.description }}</p>
          </section>

          <section class="card p-4" aria-labelledby="specs-heading" data-testid="specifications">
            <h2 id="specs-heading" class="text-sm font-semibold text-slate-800">Specifications <span class="font-normal text-slate-500">({{ specs.length }})</span></h2>
            <p v-if="specs.length === 0" class="mt-2 text-sm text-slate-500">No specifications recorded.</p>
            <table v-else class="mt-2 min-w-full text-sm">
              <tbody class="divide-y divide-slate-100">
                <tr v-for="[k, v] in specs" :key="k">
                  <th scope="row" class="w-1/3 py-1.5 pr-3 text-left align-top font-medium text-slate-600">{{ k }}</th>
                  <td class="py-1.5 break-words text-slate-800">{{ v }}</td>
                </tr>
              </tbody>
            </table>
          </section>

          <VariantsEditor :product-id="p.id" :variants="p.variants" :default-currency="p.price_currency" :can-edit="canWrite" @changed="refresh" />
          <SupplierLinks :product-id="p.id" :default-currency="p.cost_currency || p.price_currency" :can-edit="canWrite" @changed="onSuppliersChanged" />
          <ProductDuplicatesPanel :product-id="p.id" :title="p.title" :can-merge="auth.can('products.delete') && canWrite" @merged="onMerged" />
        </div>

        <div class="min-w-0 space-y-6">
          <ReviewPanel :product-id="p.id" :review-status="p.review_status" :status="p.status" @reviewed="onReviewed" />

          <section class="card p-4" aria-labelledby="tags-heading">
            <h2 id="tags-heading" class="mb-2 text-sm font-semibold text-slate-800">Tags</h2>
            <TagEditor entity-type="product" :entity-id="p.id" :tags="p.tags" :can-edit="canWrite" @updated="onTags" />
          </section>

          <ProvenancePanel :product="p" />

          <section v-if="Object.keys(customFields).length || p.internal_notes" class="card space-y-3 p-4" aria-labelledby="more-heading">
            <h2 id="more-heading" class="text-sm font-semibold text-slate-800">More details</h2>
            <CustomFieldsEditor :model-value="customFields" entity-type="product" readonly />
            <div v-if="p.internal_notes">
              <p class="text-xs font-medium text-slate-500">Internal notes</p>
              <p class="text-sm whitespace-pre-wrap text-slate-800">{{ p.internal_notes }}</p>
            </div>
          </section>
        </div>
      </div>

      <div class="mt-6">
        <RecordPanels entity-type="product" :entity-id="p.id" />
      </div>

      <ProductFormModal :open="editing" :product="p" @close="editing = false" @saved="onSaved" />
    </template>
  </div>
</template>
