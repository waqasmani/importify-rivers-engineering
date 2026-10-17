<script setup lang="ts">
// Product variants: list with add, edit and delete.
import { computed, ref } from 'vue'
import { Pencil, Plus, Trash2 } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import UiButton from '@/components/ui/UiButton.vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import VariantFormModal from './VariantFormModal.vue'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { formatMoney } from '@/lib/format'
import { stringMap } from './product-utils'

type Variant = Schemas['ProductVariant']

const props = withDefaults(defineProps<{ productId: string; variants: Variant[]; defaultCurrency?: string | null; canEdit?: boolean }>(), { defaultCurrency: null, canEdit: false })
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
            <td class="table-td text-right whitespace-nowrap">{{ formatMoney(v.price, v.currency) }}</td>
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
