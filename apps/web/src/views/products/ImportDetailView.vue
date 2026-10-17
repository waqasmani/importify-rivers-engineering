<script setup lang="ts">
// One product import: what was submitted, how REA normalized it, duplicates it matched and, while it
// is parked, the decision to apply.
import { computed } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { AlertTriangle } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import PageHeader from '@/components/ui/PageHeader.vue'
import StateViews from '@/components/ui/StateViews.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import JsonView from '@/components/products/JsonView.vue'
import PayloadComparison from '@/components/products/PayloadComparison.vue'
import DuplicateList from '@/components/products/DuplicateList.vue'
import ImportResolveForm from '@/components/products/ImportResolveForm.vue'
import ImportPricingSummary from '@/components/products/ImportPricingSummary.vue'
import { useAuth } from '@/stores/auth'
import { useDeviceNames } from '@/components/products/useDeviceNames'
import { asRecord, dupSummaries, needsDecision, safeHref, str, DECISION_LABELS, IMPORT_STATUS_LABELS } from '@/components/products/product-utils'
import { formatDateTime, titleCase } from '@/lib/format'

const props = defineProps<{ id: string }>()
const auth = useAuth()
const qc = useQueryClient()
const { deviceName } = useDeviceNames()

const q = useQuery({
  queryKey: computed(() => ['import', props.id]),
  queryFn: ({ signal }) => api.get<Schemas['ImportDetail']>(`/imports/${props.id}`, undefined, signal),
})
const imp = computed(() => q.data.value)
const candidates = computed(() => dupSummaries(imp.value?.duplicate_candidates))
const title = computed(() => {
  const n = asRecord(imp.value?.normalized)
  const p = asRecord(imp.value?.payload)
  return str(n.title) || str(p.title) || 'Product import'
})
const sourceUrl = computed(() => str(asRecord(imp.value?.normalized).source_url) || str(asRecord(imp.value?.payload).url))
const canResolve = computed(() => !!imp.value && needsDecision(imp.value.status) && auth.can('products.write'))

function onResolved(updated: Schemas['ProductImport']) {
  // Keep the pricing preview; the resolve response is the import record itself.
  qc.setQueryData(['import', props.id], { ...imp.value, ...updated })
  void qc.invalidateQueries({ queryKey: ['imports'] })
  void qc.invalidateQueries({ queryKey: ['products'] })
}
</script>

<template>
  <div>
    <StateViews v-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <template v-else-if="imp">
      <PageHeader :title="title" back="/imports">
        <template #meta>
          <div class="mt-2 flex flex-wrap items-center gap-2 text-sm text-slate-600">
            <UiBadge :value="imp.status">{{ IMPORT_STATUS_LABELS[imp.status] ?? titleCase(imp.status) }}</UiBadge>
            <span>{{ imp.source === 'csv' ? 'CSV' : imp.source === 'api' ? 'API' : titleCase(imp.source) }} import</span>
            <span>· {{ formatDateTime(imp.created_at) }}</span>
            <span v-if="imp.device_id">· {{ deviceName(imp.device_id) }}</span>
            <span v-if="imp.decision">· Decision: {{ DECISION_LABELS[imp.decision] ?? imp.decision }}</span>
            <span v-if="imp.processed_at">· Processed {{ formatDateTime(imp.processed_at) }}</span>
          </div>
        </template>
        <template #actions>
          <RouterLink v-if="imp.product_id" :to="`/products/${imp.product_id}`" class="rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700" data-testid="import-product-link">Open product</RouterLink>
          <RouterLink v-if="imp.captured_page_id" :to="`/captures/${imp.captured_page_id}`" class="rounded-md border border-slate-300 bg-white px-3.5 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50">Captured page</RouterLink>
        </template>
      </PageHeader>

      <div class="space-y-6">
        <section v-if="canResolve" class="card space-y-3 border-amber-300 p-4 sm:p-5" aria-labelledby="resolve-heading">
          <h2 id="resolve-heading" class="flex items-center gap-2 text-base font-semibold text-slate-900"><AlertTriangle class="h-4 w-4 text-amber-600" aria-hidden="true" /> Decision needed</h2>
          <p class="text-sm text-slate-600">REA parked this import because it looks like a product that already exists. Choose what to do. A new product is always a draft pending review.</p>
          <ImportResolveForm :import-id="imp.id" :candidates="candidates" :default-target="imp.duplicate_of_id" @resolved="onResolved" />
        </section>
        <p v-else-if="needsDecision(imp.status)" class="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900">This import is waiting for a decision by someone who can edit products.</p>

        <div v-if="imp.error" class="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700" role="alert">
          <p class="font-medium">Import error</p>
          <p>{{ imp.error }}</p>
        </div>

        <section v-if="imp.warnings.length" class="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900" aria-label="Warnings">
          <p class="font-medium">Warnings</p>
          <ul class="mt-1 list-disc pl-5">
            <li v-for="w in imp.warnings" :key="w">{{ w }}</li>
          </ul>
        </section>

        <section class="card space-y-3 p-4 sm:p-5" aria-labelledby="compare-heading">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <h2 id="compare-heading" class="text-base font-semibold text-slate-900">Submitted vs normalized</h2>
            <a v-if="safeHref(sourceUrl)" :href="safeHref(sourceUrl)" target="_blank" rel="noopener noreferrer nofollow" class="max-w-full truncate text-sm text-brand-700 hover:underline">{{ sourceUrl }}</a>
          </div>
          <p v-if="!imp.normalized" class="text-sm text-slate-500">This import has no normalized data.</p>
          <PayloadComparison :payload="imp.payload" :normalized="imp.normalized" />
          <div class="grid gap-3 lg:grid-cols-2">
            <JsonView :value="imp.payload" summary="Raw payload (JSON)" />
            <JsonView :value="imp.normalized" summary="Normalized (JSON)" />
          </div>
        </section>

        <section v-if="imp.pricing" class="card p-4 sm:p-5" aria-label="Pricing">
          <ImportPricingSummary
            :pricing="imp.pricing"
            title="Selling price with the current rules"
            :note="imp.product_id ? 'Recalculated now from the captured source price. The stored product may differ if its price is locked or its cost was entered by hand; open the product for its actual price.' : 'What the product would cost and sell for if it were imported now.'"
          />
        </section>

        <section class="card space-y-3 p-4 sm:p-5" aria-labelledby="dups-heading">
          <h2 id="dups-heading" class="text-base font-semibold text-slate-900">Duplicate candidates</h2>
          <DuplicateList :candidates="candidates" legend="Products this import matched when it was submitted" />
          <p v-if="imp.duplicate_of_id && !candidates.some((c) => c.id === imp?.duplicate_of_id)" class="text-sm">
            Best match: <RouterLink :to="`/products/${imp.duplicate_of_id}`" class="text-brand-700 hover:underline">open product</RouterLink>
          </p>
        </section>
      </div>
    </template>
  </div>
</template>
