<script setup lang="ts">
// Other products that look like this one, with a merge into this product.
import { computed, ref } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { api, errorMessage, type Schemas } from '@/api/client'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import StateViews from '@/components/ui/StateViews.vue'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { formatMoney } from '@/lib/format'
import { asRecord, scoreLabel, str } from './product-utils'

const props = withDefaults(defineProps<{ productId: string; title: string; canMerge?: boolean }>(), { canMerge: false })
const emit = defineEmits<{ merged: [product: Schemas['ProductDetail']] }>()
const toast = useToast()
const { confirm } = useConfirm()

const q = useQuery({
  queryKey: computed(() => ['product-duplicates', props.productId]),
  queryFn: ({ signal }) => api.get<Schemas['DuplicateCandidateItems']>(`/products/${props.productId}/duplicates`, undefined, signal),
})
const items = computed(() => q.data.value?.items ?? [])
const selected = ref<string[]>([])
const merging = ref(false)

function rec(c: Schemas['DuplicateCandidate']) {
  const r = asRecord(c.record)
  return { domain: str(r.source_domain), sku: str(r.sku), price: formatMoney(str(r.price) || null, str(r.price_currency) || null), review: str(r.review_status) }
}

async function merge() {
  const chosen = items.value.filter((c) => selected.value.includes(c.id))
  if (chosen.length === 0) return
  const ok = await confirm({
    title: `Merge ${chosen.length} product${chosen.length === 1 ? '' : 's'} into this one`,
    message: `The selected products are deleted after their variants, images, supplier and opportunity links, imports, notes, files, tags and tasks move to “${props.title}”. Empty fields on this product are filled from them. This cannot be undone.`,
    details: chosen.map((c) => c.label),
    confirmLabel: 'Merge and delete',
    danger: true,
  })
  if (!ok) return
  merging.value = true
  try {
    const res = await api.post<Schemas['ProductDetail']>(`/products/${props.productId}/merge`, { source_ids: chosen.map((c) => c.id) } satisfies Schemas['MergeInput'])
    selected.value = []
    toast.success('Products merged.')
    await q.refetch()
    emit('merged', res)
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    merging.value = false
  }
}
</script>

<template>
  <section class="card p-4" aria-labelledby="dups-heading" data-testid="product-duplicates">
    <div class="mb-3 flex items-center justify-between gap-2">
      <h2 id="dups-heading" class="text-sm font-semibold text-slate-800">Possible duplicates</h2>
      <UiButton v-if="canMerge && items.length" size="sm" variant="danger" :disabled="selected.length === 0" :loading="merging" @click="merge">Merge selected into this</UiButton>
    </div>
    <StateViews v-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <p v-else-if="items.length === 0" class="text-sm text-slate-500">No similar products found.</p>
    <ul v-else class="divide-y divide-slate-100">
      <li v-for="c in items" :key="c.id" class="flex items-start gap-3 py-2">
        <input v-if="canMerge" v-model="selected" type="checkbox" :value="c.id" class="mt-1 rounded border-slate-300" :aria-label="`Select ${c.label} for merging`" />
        <div class="min-w-0 flex-1">
          <div class="flex flex-wrap items-center gap-2">
            <RouterLink :to="`/products/${c.id}`" class="truncate text-sm font-medium text-brand-700 hover:underline">{{ c.label }}</RouterLink>
            <UiBadge :tone="c.score >= 0.9 ? 'red' : c.score >= 0.7 ? 'amber' : 'slate'">{{ scoreLabel(c.score) }} match</UiBadge>
            <UiBadge v-if="rec(c).review" :value="rec(c).review" />
          </div>
          <p class="text-xs text-slate-500">{{ c.reasons.join(' · ') }}</p>
          <p class="text-xs text-slate-500">{{ [rec(c).domain, rec(c).sku, rec(c).price !== '—' ? rec(c).price : ''].filter(Boolean).join(' · ') }}</p>
        </div>
      </li>
    </ul>
  </section>
</template>
