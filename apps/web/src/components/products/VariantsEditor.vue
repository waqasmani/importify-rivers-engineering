<script setup lang="ts">
// Product variants: list with add, edit and delete, and each variant's source price, cost and
// selling price (set by the pricing rules unless locked by hand).
import { computed, ref } from 'vue'
import { Lock, Pencil, Plus, Trash2 } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import VariantFormModal from './VariantFormModal.vue'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { formatMoney } from '@/lib/format'
import { formatAmount } from '@/components/pricing/pricing-utils'
import { stringMap } from './product-utils'

type Variant = Schemas['ProductVariant']

const props = withDefaults(
  defineProps<{ productId: string; variants: Variant[]; plans?: Schemas['VariantPricingPlan'][]; defaultCurrency?: string | null; canEdit?: boolean }>(),
  { plans: () => [], defaultCurrency: null, canEdit: false },
)
const planFor = (id: string) => props.plans.find((x) => x.variant_id === id)?.plan
const emit = defineEmits<{ changed: [] }>()
const toast = useToast()
const { confirm } = useConfirm()

const sorted = computed(() => [...props.variants].sort((a, b) => a.position - b.position || a.title.localeCompare(b.title)))
const nextPosition = computed(() => (props.variants.length ? Math.max(...props.variants.map((v) => v.position)) + 1 : 0))
const modal = ref(false)
const editing = ref<Variant | null>(null)
const deleting = ref<string | null>(null)

function open(v: Variant | null) {
  editing.value = v
  modal.value = true
}

function name(v: Variant) {
  return v.title || v.sku || Object.values(stringMap(v.options)).join(' / ') || 'Variant'
}

async function remove(v: Variant) {
  const ok = await confirm({ title: 'Delete variant', message: `Delete the variant “${name(v)}”? This cannot be undone.`, confirmLabel: 'Delete', danger: true })
  if (!ok) return
  deleting.value = v.id
  try {
    await api.del(`/products/${props.productId}/variants/${v.id}`)
    toast.success('Variant deleted.')
    emit('changed')
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    deleting.value = null
  }
}
</script>

<template>
  <section class="card p-4" aria-labelledby="variants-heading" data-testid="variants">
    <div class="mb-3 flex items-center justify-between gap-2">
      <h2 id="variants-heading" class="text-sm font-semibold text-slate-800">Variants <span class="font-normal text-slate-500">({{ variants.length }})</span></h2>
      <UiButton v-if="canEdit" size="sm" @click="open(null)"><Plus class="h-4 w-4" /> Add variant</UiButton>
    </div>
    <p v-if="sorted.length === 0" class="text-sm text-slate-500">No variants recorded.</p>
    <div v-else class="relative overflow-x-auto">
      <table class="min-w-full divide-y divide-slate-200 text-sm">
        <thead>
          <tr>
            <th scope="col" class="table-th">Variant</th>
            <th scope="col" class="table-th">Options</th>
            <th scope="col" class="table-th hidden text-right md:table-cell">Source</th>
            <th scope="col" class="table-th hidden text-right sm:table-cell">Cost</th>
            <th scope="col" class="table-th text-right">Price</th>
            <th scope="col" class="table-th">Availability</th>
            <th v-if="canEdit" scope="col" class="table-th"><span class="sr-only">Actions</span></th>
          </tr>
        </thead>
        <tbody class="divide-y divide-slate-100">
          <tr v-for="v in sorted" :key="v.id" :data-testid="`variant-${v.id}`">
            <td class="table-td">
              <div class="flex items-center gap-2">
                <img v-if="v.image_url" :src="v.image_url" alt="" loading="lazy" referrerpolicy="no-referrer" class="h-8 w-8 rounded border border-slate-200 object-cover" />
                <div class="min-w-0">
                  <p class="font-medium text-slate-800">{{ name(v) }}</p>
                  <p v-if="v.sku && v.title" class="text-xs text-slate-500">SKU {{ v.sku }}</p>
                </div>
              </div>
            </td>
            <td class="table-td">
              <span v-for="(val, k) in stringMap(v.options)" :key="k" class="mr-1 mb-1 inline-block rounded bg-slate-100 px-1.5 py-0.5 text-xs text-slate-700">{{ k }}: {{ val }}</span>
              <span v-if="Object.keys(stringMap(v.options)).length === 0" class="text-slate-400">—</span>
            </td>
            <td class="table-td hidden text-right whitespace-nowrap text-slate-500 md:table-cell">{{ formatMoney(v.source_price, v.source_currency) }}</td>
            <td class="table-td hidden text-right whitespace-nowrap sm:table-cell">
              <template v-if="v.cost">{{ formatAmount(v.cost, v.cost_currency) }}<span v-if="v.cost_manual" class="block text-xs text-slate-500">by hand</span></template>
              <span v-else class="text-xs text-slate-500">product cost</span>
            </td>
            <td class="table-td text-right whitespace-nowrap" :data-testid="`variant-price-${v.id}`">
              <span class="inline-flex items-center gap-1">
                <Lock v-if="v.price_locked" class="h-3.5 w-3.5 text-sky-600" aria-label="Locked manual price" />
                {{ formatMoney(v.price, v.currency) }}
              </span>
              <s v-if="v.compare_at_price" class="block text-xs text-slate-500">{{ formatMoney(v.compare_at_price, v.currency) }}</s>
              <span v-if="planFor(v.id)?.changed && planFor(v.id)?.status === 'repriced'" class="block text-xs text-amber-700">rules: {{ formatMoney(planFor(v.id)?.evaluation.price, planFor(v.id)?.evaluation.currency) }}</span>
            </td>
            <td class="table-td"><UiBadge :value="v.availability" /></td>
            <td v-if="canEdit" class="table-td text-right whitespace-nowrap">
              <button type="button" class="rounded p-1.5 text-slate-400 hover:bg-slate-100 hover:text-slate-700" :aria-label="`Edit variant ${name(v)}`" @click="open(v)"><Pencil class="h-4 w-4" /></button>
              <button type="button" class="rounded p-1.5 text-slate-400 hover:bg-red-50 hover:text-red-600 disabled:opacity-50" :disabled="deleting === v.id" :aria-label="`Delete variant ${name(v)}`" @click="remove(v)"><Trash2 class="h-4 w-4" /></button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <VariantFormModal :open="modal" :product-id="productId" :variant="editing" :default-currency="defaultCurrency" :next-position="nextPosition" @close="modal = false" @saved="emit('changed')" />
  </section>
</template>
