<script setup lang="ts">
// Bulk reprice: a dry run shows the before/after of every product and variant, then the new
// prices can be applied. Locked manual prices are never changed. Only REA's own records change.
import { computed, ref, watch } from 'vue'
import { ArrowRight, Lock } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import UiModal from '@/components/ui/UiModal.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import StateViews from '@/components/ui/StateViews.vue'
import { useToast } from '@/composables/useToast'
import { formatMoney } from '@/lib/format'
import { PRICING_STATUS, formatAmount, sameSnapshot, snapshotPrice, statusTone } from '@/components/pricing/pricing-utils'

const props = withDefaults(defineProps<{ open: boolean; ids?: string[]; filter?: Schemas['RepriceFilter'] | null; noun?: string }>(), {
  ids: () => [],
  filter: null,
  noun: 'products',
})
const emit = defineEmits<{ close: []; applied: [result: Schemas['RepriceResult']] }>()
const toast = useToast()

const preview = ref<Schemas['RepriceResult'] | null>(null)
const loading = ref(false)
const applying = ref(false)
const error = ref('')
const onlyChanges = ref(true)

function body(dryRun: boolean): Schemas['RepriceInput'] {
  return props.filter ? { filter: props.filter, dry_run: dryRun } : { ids: props.ids, dry_run: dryRun }
}

async function load() {
  loading.value = true
  error.value = ''
  preview.value = null
  try {
    preview.value = await api.post<Schemas['RepriceResult']>('/products/reprice', body(true))
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
watch(
  () => props.open,
  (open) => {
    if (open) void load()
  },
  { immediate: true },
)

const items = computed(() => {
  const all = preview.value?.items ?? []
  return onlyChanges.value ? all.filter((i) => i.changed || i.variants.some((v) => v.changed)) : all
})
const summary = computed(() => preview.value?.summary)

async function apply() {
  applying.value = true
  error.value = ''
  try {
    const res = await api.post<Schemas['RepriceResult']>('/products/reprice', body(false))
    toast.success(`Repriced ${res.summary.changed} of ${res.summary.products} ${props.noun}. Nothing was published or sent anywhere.`)
    emit('applied', res)
    emit('close')
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    applying.value = false
  }
}
</script>

<template>
  <UiModal :open="open" :title="`Reprice ${noun}`" size="xl" @close="emit('close')">
    <div class="space-y-4" data-testid="reprice-dialog">
      <p class="text-sm text-slate-600">
        A dry run with the current pricing rules and exchange rates. Costs derived from source prices are refreshed; locked manual prices stay as they are. Review the changes, then apply them to REA's records.
      </p>
      <StateViews v-if="loading" kind="loading" message="Calculating new prices…" />
      <p v-else-if="error" class="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700" role="alert">{{ error }}</p>
      <template v-if="preview && summary">
        <dl class="grid grid-cols-2 gap-2 text-sm sm:grid-cols-5" data-testid="reprice-summary">
          <div class="rounded-md bg-slate-50 px-3 py-2"><dt class="text-xs text-slate-500">Products</dt><dd class="font-semibold">{{ summary.products }}</dd></div>
          <div class="rounded-md bg-amber-50 px-3 py-2"><dt class="text-xs text-amber-800">Will change</dt><dd class="font-semibold" data-testid="summary-changed">{{ summary.changed }}</dd></div>
          <div class="rounded-md bg-slate-50 px-3 py-2"><dt class="text-xs text-slate-500">Unchanged</dt><dd class="font-semibold">{{ summary.unchanged }}</dd></div>
          <div class="rounded-md bg-sky-50 px-3 py-2"><dt class="text-xs text-sky-800">Locked</dt><dd class="font-semibold">{{ summary.locked }}</dd></div>
          <div class="rounded-md bg-slate-50 px-3 py-2"><dt class="text-xs text-slate-500">Not priced</dt><dd class="font-semibold">{{ summary.not_priced }}</dd></div>
        </dl>
        <p class="text-xs text-slate-500">{{ summary.variants_changed }} of {{ summary.variants }} variant prices change. Prices are in {{ preview.base_currency }} unless shown otherwise.</p>
        <label class="flex items-center gap-2 text-sm text-slate-700"><input v-model="onlyChanges" type="checkbox" class="rounded border-slate-300" /> Show only products that change</label>

        <p v-if="items.length === 0" class="text-sm text-slate-500">{{ onlyChanges ? 'Nothing would change.' : 'No products.' }}</p>
        <div v-else class="max-h-[50vh] overflow-auto rounded-md border border-slate-200">
          <table class="min-w-full divide-y divide-slate-200 text-sm">
            <thead class="sticky top-0 bg-slate-50">
              <tr>
                <th scope="col" class="table-th">Product</th>
                <th scope="col" class="table-th">Status</th>
                <th scope="col" class="table-th text-right">Cost</th>
                <th scope="col" class="table-th text-right">Price</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-slate-100 bg-white">
              <template v-for="it in items" :key="it.product_id">
                <tr :data-testid="`reprice-${it.product_id}`">
                  <td class="table-td">
                    <RouterLink :to="`/products/${it.product_id}`" class="font-medium text-brand-700 hover:underline" target="_blank">{{ it.title }}</RouterLink>
                    <p v-if="it.message" class="text-xs text-slate-500">{{ it.message }}</p>
                    <p v-if="it.after.pricing_rule_name && it.status === 'repriced'" class="text-xs text-slate-500">Rule: {{ it.after.pricing_rule_name }}</p>
                  </td>
                  <td class="table-td">
                    <UiBadge :tone="statusTone(it.status)"><Lock v-if="it.status === 'locked'" class="h-3 w-3" aria-hidden="true" /> {{ PRICING_STATUS[it.status] ?? it.status }}</UiBadge>
                  </td>
                  <td class="table-td text-right whitespace-nowrap">
                    <template v-if="it.before.cost !== it.after.cost || it.before.cost_currency !== it.after.cost_currency">
                      <span class="text-slate-500 line-through">{{ formatAmount(it.before.cost, it.before.cost_currency) }}</span>
                      <ArrowRight class="mx-1 inline h-3 w-3 text-slate-400" aria-label="becomes" />
                    </template>
                    {{ formatAmount(it.after.cost, it.after.cost_currency) }}
                  </td>
                  <td class="table-td text-right whitespace-nowrap" data-testid="reprice-price">
                    <template v-if="!sameSnapshot(it.before, it.after) && it.before.price !== it.after.price">
                      <span class="text-slate-500 line-through">{{ snapshotPrice(it.before) }}</span>
                      <ArrowRight class="mx-1 inline h-3 w-3 text-slate-400" aria-label="becomes" />
                    </template>
                    <span class="font-medium" :class="it.before.price !== it.after.price ? 'text-amber-800' : 'text-slate-900'">{{ snapshotPrice(it.after) }}</span>
                    <span v-if="it.after.compare_at_price" class="block text-xs text-slate-500">compare-at {{ formatMoney(it.after.compare_at_price, it.after.price_currency) }}</span>
                  </td>
                </tr>
                <tr v-for="v in it.variants.filter((x) => !onlyChanges || x.changed)" :key="v.variant_id" class="bg-slate-50/60" :data-testid="`reprice-variant-${v.variant_id}`">
                  <td class="table-td pl-8 text-slate-600">↳ {{ v.title || 'Variant' }}</td>
                  <td class="table-td"><UiBadge :tone="statusTone(v.status)">{{ PRICING_STATUS[v.status] ?? v.status }}</UiBadge></td>
                  <td class="table-td text-right whitespace-nowrap text-slate-600">{{ v.after.cost ? formatAmount(v.after.cost, v.after.cost_currency) : 'product cost' }}</td>
                  <td class="table-td text-right whitespace-nowrap">
                    <template v-if="v.before.price !== v.after.price">
                      <span class="text-slate-500 line-through">{{ snapshotPrice(v.before) }}</span>
                      <ArrowRight class="mx-1 inline h-3 w-3 text-slate-400" aria-label="becomes" />
                    </template>
                    <span :class="v.before.price !== v.after.price ? 'text-amber-800' : ''">{{ snapshotPrice(v.after) }}</span>
                  </td>
                </tr>
              </template>
            </tbody>
          </table>
        </div>
      </template>
    </div>
    <template #footer>
      <UiButton variant="ghost" @click="emit('close')">Cancel</UiButton>
      <UiButton variant="primary" :loading="applying" :disabled="!preview || !summary || summary.changed === 0" data-testid="reprice-apply" @click="apply">
        Apply new prices<template v-if="summary"> ({{ summary.changed }})</template>
      </UiButton>
    </template>
  </UiModal>
</template>
