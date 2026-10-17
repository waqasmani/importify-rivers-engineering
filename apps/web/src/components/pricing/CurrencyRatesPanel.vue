<script setup lang="ts">
// Manually maintained exchange rates against the base currency (Settings → General → default
// currency). REA never fetches rates from the internet; operators can also update them through
// PUT /api/v1/pricing/rates with a personal API token.
import { computed, reactive, ref } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { Pencil, Trash2 } from 'lucide-vue-next'
import { api, buildQuery, errorMessage, type Schemas } from '@/api/client'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import StateViews from '@/components/ui/StateViews.vue'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { formatDateTime } from '@/lib/format'
import { dec } from './pricing-utils'

type Rate = Schemas['CurrencyRate']

const qc = useQueryClient()
const toast = useToast()
const { confirm } = useConfirm()
const { errors, capture, clear } = useFieldErrors()
const q = useQuery({ queryKey: ['pricing', 'rates'], queryFn: ({ signal }) => api.get<Schemas['CurrencyRatesDoc']>('/pricing/rates', undefined, signal) })
const base = computed(() => q.data.value?.base_currency ?? '')
const form = reactive({ currency: '', rate: '', note: '' })
const editing = ref<string | null>(null)
const saving = ref(false)

function inverse(rate: string): string {
  const n = Number(rate)
  return n > 0 ? (1 / n).toFixed(4) : '—'
}
function edit(r: Rate) {
  clear()
  editing.value = r.currency
  form.currency = r.currency
  form.rate = r.rate
  form.note = r.note
}
function reset() {
  editing.value = null
  form.currency = ''
  form.rate = ''
  form.note = ''
  clear()
}

async function save() {
  saving.value = true
  clear()
  const currency = form.currency.trim().toUpperCase()
  try {
    await api.put<Rate>(`/pricing/rates/${encodeURIComponent(currency || '-')}`, { rate: dec(form.rate) ?? '', note: form.note.trim() } satisfies Schemas['RateInput'])
    toast.success(`Saved 1 ${currency} = ${dec(form.rate)} ${base.value}. Reprice products to apply it to stored costs.`)
    reset()
    await qc.invalidateQueries({ queryKey: ['pricing', 'rates'] })
  } catch (e) {
    capture(e)
  } finally {
    saving.value = false
  }
}

async function remove(r: Rate) {
  const ok = await confirm({
    title: 'Delete exchange rate',
    message: `Delete the rate for ${r.currency}? Products whose cost is in ${r.currency} can no longer be priced until a rate is added again.`,
    confirmLabel: 'Delete rate',
    danger: true,
  })
  if (!ok) return
  try {
    await api.del(`/pricing/rates/${r.currency}${r.base_currency !== base.value ? buildQuery({ base: r.base_currency }) : ''}`)
    toast.success(`Deleted the ${r.currency} rate.`)
    if (editing.value === r.currency) reset()
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    await qc.invalidateQueries({ queryKey: ['pricing', 'rates'] })
  }
}
</script>

<template>
  <div class="space-y-4" data-testid="currency-rates">
    <StateViews v-if="q.isLoading.value" kind="loading" />
    <StateViews v-else-if="q.error.value" kind="error" :message="errorMessage(q.error.value)" @retry="q.refetch()" />
    <template v-else-if="q.data.value">
      <p class="text-sm text-slate-600">
        Costs in other currencies are converted into <strong>{{ base }}</strong> (the default currency in General settings) with these rates. Enter how many {{ base }} one unit of the currency is worth.
      </p>
      <div v-if="q.data.value.items.length" class="overflow-x-auto rounded-md border border-slate-200">
        <table class="min-w-full divide-y divide-slate-200 text-sm">
          <thead class="bg-slate-50">
            <tr>
              <th scope="col" class="table-th">Currency</th>
              <th scope="col" class="table-th text-right">Rate</th>
              <th scope="col" class="table-th hidden sm:table-cell">Note</th>
              <th scope="col" class="table-th hidden md:table-cell">Updated</th>
              <th scope="col" class="table-th"><span class="sr-only">Actions</span></th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100 bg-white">
            <tr v-for="r in q.data.value.items" :key="r.currency" :data-testid="`rate-${r.currency}`">
              <td class="table-td font-medium">{{ r.currency }}</td>
              <td class="table-td text-right whitespace-nowrap">
                <span class="font-mono">1 {{ r.currency }} = {{ r.rate }} {{ base }}</span>
                <span class="block text-xs text-slate-500">1 {{ base }} = {{ inverse(r.rate) }} {{ r.currency }}</span>
              </td>
              <td class="table-td hidden text-slate-500 sm:table-cell">{{ r.note || '—' }}</td>
              <td class="table-td hidden whitespace-nowrap text-slate-500 md:table-cell">{{ formatDateTime(r.updated_at) }}</td>
              <td class="table-td text-right whitespace-nowrap">
                <button type="button" class="rounded p-1.5 text-slate-400 hover:bg-slate-100 hover:text-slate-700" :aria-label="`Edit the ${r.currency} rate`" @click="edit(r)"><Pencil class="h-4 w-4" /></button>
                <button type="button" class="rounded p-1.5 text-slate-400 hover:bg-red-50 hover:text-red-600" :aria-label="`Delete the ${r.currency} rate`" @click="remove(r)"><Trash2 class="h-4 w-4" /></button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <p v-else class="text-sm text-slate-500">No rates yet. Products bought in {{ base }} need none; add a rate for every other currency your suppliers use.</p>

      <form class="grid gap-3 rounded-md border border-slate-200 bg-slate-50 p-3 sm:grid-cols-[7rem_1fr_1fr_auto] sm:items-end" data-testid="rate-form" @submit.prevent="save">
        <FormField v-slot="{ id }" label="Currency" :error="errors.currency">
          <input :id="id" v-model="form.currency" class="input uppercase" maxlength="3" placeholder="EUR" required :disabled="!!editing" data-testid="rate-currency" />
        </FormField>
        <FormField v-slot="{ id }" :label="`Value of 1 unit in ${base}`" :error="errors.rate">
          <input :id="id" v-model="form.rate" type="number" min="0" step="any" class="input" placeholder="1.0850" required data-testid="rate-value" />
        </FormField>
        <FormField v-slot="{ id }" label="Note" :error="errors.note">
          <input :id="id" v-model="form.note" class="input" maxlength="300" placeholder="Source and date" />
        </FormField>
        <div class="flex gap-2">
          <UiButton type="submit" variant="primary" :loading="saving">{{ editing ? 'Save rate' : 'Add rate' }}</UiButton>
          <UiButton v-if="editing" variant="ghost" @click="reset">Cancel</UiButton>
        </div>
      </form>

      <div v-if="q.data.value.other_bases.length" class="rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900" data-testid="stale-rates">
        <p class="font-medium">Rates recorded against another base currency</p>
        <p class="text-xs">They are not used while {{ base }} is the base currency.</p>
        <ul class="mt-2 space-y-1">
          <li v-for="r in q.data.value.other_bases" :key="`${r.base_currency}-${r.currency}`" class="flex items-center justify-between gap-2">
            <span class="font-mono text-xs">1 {{ r.currency }} = {{ r.rate }} {{ r.base_currency }}</span>
            <UiButton size="sm" variant="ghost" :aria-label="`Delete the ${r.currency} rate against ${r.base_currency}`" @click="remove(r)"><Trash2 class="h-4 w-4" /> Delete</UiButton>
          </li>
        </ul>
      </div>
    </template>
  </div>
</template>
