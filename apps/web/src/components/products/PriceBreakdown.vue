<script setup lang="ts">
// Price breakdown of one product: source price → cost → rule → selling price, compare-at price and
// margin, with the live plan from the current rules. A manual price can be locked (rules never
// change it) or handed back to the rules, and the price can be recalculated on demand.
import { computed, reactive, ref } from 'vue'
import { ArrowRight, Lock, LockOpen, RefreshCw } from 'lucide-vue-next'
import { api, errorMessage, type Schemas } from '@/api/client'
import UiBadge from '@/components/ui/UiBadge.vue'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import PricingEvaluationView from '@/components/pricing/PricingEvaluationView.vue'
import { useFieldErrors } from '@/composables/useFieldErrors'
import { useToast } from '@/composables/useToast'
import { useConfirm } from '@/composables/useConfirm'
import { formatDateTime, formatMoney } from '@/lib/format'
import { PRICING_STATUS, dec, formatAmount, statusTone } from '@/components/pricing/pricing-utils'

type Detail = Schemas['ProductDetail']

const props = withDefaults(defineProps<{ product: Detail; canEdit?: boolean }>(), { canEdit: false })
const emit = defineEmits<{ updated: [product: Detail]; repriced: [] }>()
const toast = useToast()
const { confirm } = useConfirm()
const { errors, capture, clear } = useFieldErrors()

const plan = computed(() => props.product.pricing.product)
const ev = computed(() => plan.value.evaluation)
const base = computed(() => props.product.pricing.base_currency)
const locking = ref(false)
const busy = ref<'lock' | 'unlock' | 'recalc' | null>(null)
const lockForm = reactive({ price: '', currency: '', compare_at: '' })

const marginText = computed(() => {
  const p = props.product
  if (p.margin_percent !== null) return `${Number(p.margin_percent).toFixed(2)}% · ${formatMoney(p.margin_amount, p.price_currency)}`
  if (p.price && p.cost && p.price_currency !== p.cost_currency) return 'Not calculated: price and cost use different currencies.'
  if (!p.price || !p.cost) return 'Needs both a price and a cost.'
  return '—'
})
const costOrigin = computed(() => {
  const p = props.product
  if (!p.cost) return ''
  if (p.cost_manual) return 'entered by hand'
  if (p.source_price) return p.cost_currency === p.source_currency ? 'from the source price' : `converted from the source price`
  return ''
})
const ruleName = computed(() => props.product.pricing_rule_name ?? (props.product.pricing_rule_id ? props.product.pricing.rule_names[props.product.pricing_rule_id] : undefined))
const pendingChange = computed(() => !props.product.price_locked && plan.value.changed && ev.value.status === 'priced')

function startLock() {
  clear()
  lockForm.price = props.product.price ?? ev.value.price ?? ''
  lockForm.currency = props.product.price_currency ?? base.value
  lockForm.compare_at = props.product.compare_at_price ?? ''
  locking.value = true
}

async function saveLock() {
  busy.value = 'lock'
  clear()
  try {
    const body: Schemas['ProductPricingInput'] = {
      price_locked: true,
      price: dec(lockForm.price),
      price_currency: lockForm.currency.trim().toUpperCase() || null,
      compare_at_price: dec(lockForm.compare_at),
    }
    const d = await api.put<Detail>(`/products/${props.product.id}/pricing`, body)
    toast.success('Manual price locked. Pricing rules will not change it.')
    locking.value = false
    emit('updated', d)
  } catch (e) {
    capture(e)
  } finally {
    busy.value = null
  }
}

async function unlock() {
  const rulePrice = ev.value.status === 'priced' ? formatMoney(ev.value.price, ev.value.currency) : null
  const ok = await confirm({
    title: 'Let the pricing rules set the price',
    message: rulePrice
      ? `The manual price ${formatMoney(props.product.price, props.product.price_currency)} is replaced by ${rulePrice} from the rule “${ev.value.rule?.name ?? ''}”.`
      : `The price stays ${formatMoney(props.product.price, props.product.price_currency)} for now: ${ev.value.message}`,
    confirmLabel: 'Unlock price',
  })
  if (!ok) return
  busy.value = 'unlock'
  try {
    const d = await api.put<Detail>(`/products/${props.product.id}/pricing`, { price_locked: false } satisfies Schemas['ProductPricingInput'])
    toast.success('The pricing rules set this price again.')
    emit('updated', d)
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    busy.value = null
  }
}

async function recalculate() {
  busy.value = 'recalc'
  try {
    const res = await api.post<Schemas['RepriceResult']>('/products/reprice', { ids: [props.product.id], dry_run: false } satisfies Schemas['RepriceInput'])
    const item = res.items[0]
    if (!item) toast.info('Nothing to recalculate.')
    else if (!item.changed && !item.variants.some((v) => v.changed)) toast.info(item.message || 'The price is already up to date with the current rules.')
    else toast.success(item.status === 'locked' ? 'Cost and variant prices updated; the locked price was kept.' : `Recalculated: ${formatMoney(item.after.price, item.after.price_currency)}.`)
    emit('repriced')
  } catch (e) {
    toast.error(errorMessage(e))
  } finally {
    busy.value = null
  }
}
</script>

<template>
  <div data-testid="price-breakdown">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <h2 id="pricing-heading" class="text-sm font-semibold text-slate-800">Pricing</h2>
      <UiBadge v-if="product.price_locked" tone="blue"><Lock class="h-3 w-3" aria-hidden="true" /> Manual price</UiBadge>
      <UiBadge v-else-if="ruleName" tone="green">Rule: {{ ruleName }}</UiBadge>
      <UiBadge v-else :tone="statusTone(plan.status)">{{ PRICING_STATUS[plan.status] ?? plan.status }}</UiBadge>
    </div>

    <ol class="mt-3 flex flex-wrap items-stretch gap-1.5 text-sm" aria-label="Price breakdown">
      <li class="min-w-0 rounded-md bg-slate-50 px-2.5 py-1.5">
        <p class="text-xs font-medium text-slate-500">Source price</p>
        <p class="font-medium text-slate-900" data-testid="source-price">{{ formatMoney(product.source_price, product.source_currency) }}</p>
      </li>
      <li class="flex items-center text-slate-300" aria-hidden="true"><ArrowRight class="h-4 w-4" /></li>
      <li class="min-w-0 rounded-md bg-slate-50 px-2.5 py-1.5">
        <p class="text-xs font-medium text-slate-500">Cost</p>
        <p class="font-medium text-slate-900" data-testid="cost">{{ formatAmount(product.cost, product.cost_currency) }}</p>
        <p v-if="costOrigin" class="text-xs text-slate-500">{{ costOrigin }}</p>
      </li>
      <li class="flex items-center text-slate-300" aria-hidden="true"><ArrowRight class="h-4 w-4" /></li>
      <li class="min-w-0 rounded-md bg-slate-50 px-2.5 py-1.5">
        <p class="text-xs font-medium text-slate-500">Rule</p>
        <p class="font-medium text-slate-900">{{ product.price_locked ? 'Locked by hand' : (ruleName ?? 'None yet') }}</p>
      </li>
      <li class="flex items-center text-slate-300" aria-hidden="true"><ArrowRight class="h-4 w-4" /></li>
      <li class="min-w-0 rounded-md bg-brand-50 px-2.5 py-1.5">
        <p class="text-xs font-medium text-brand-700">Selling price</p>
        <p class="text-lg leading-tight font-semibold text-slate-900" data-testid="selling-price">{{ formatMoney(product.price, product.price_currency) }}</p>
        <p v-if="product.compare_at_price" class="text-xs text-slate-500">compare-at <s>{{ formatMoney(product.compare_at_price, product.price_currency) }}</s></p>
      </li>
    </ol>

    <dl class="mt-3 grid grid-cols-2 gap-3 text-sm">
      <div class="col-span-2">
        <dt class="text-xs font-medium text-slate-500">Margin</dt>
        <dd :class="product.margin_percent !== null ? 'text-slate-900' : 'text-slate-500'">{{ marginText }}</dd>
      </div>
      <div v-if="product.priced_at" class="col-span-2">
        <dt class="text-xs font-medium text-slate-500">Last priced by a rule</dt>
        <dd class="text-slate-700">{{ formatDateTime(product.priced_at) }}</dd>
      </div>
    </dl>

    <p v-if="pendingChange" class="mt-3 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900" role="status" data-testid="pending-change">
      The current rules give {{ formatMoney(ev.price, ev.currency) }}<template v-if="ev.rule"> ({{ ev.rule.name }})</template>. Recalculate to apply it.
    </p>
    <p v-else-if="product.price_locked && ev.status === 'priced'" class="mt-3 text-xs text-slate-500" data-testid="locked-note">
      The rules would give {{ formatMoney(ev.price, ev.currency) }}; the locked price is kept.
    </p>
    <p v-else-if="ev.status !== 'priced' && !product.price_locked" class="mt-3 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900" role="status">{{ ev.message }}</p>

    <details class="mt-3 text-sm">
      <summary class="cursor-pointer text-brand-700">How the current rules calculate it</summary>
      <div class="mt-2"><PricingEvaluationView :evaluation="ev" compact /></div>
    </details>

    <form v-if="locking" class="mt-3 space-y-3 rounded-md border border-slate-200 bg-slate-50 p-3" data-testid="lock-form" @submit.prevent="saveLock">
      <div class="grid grid-cols-[1fr_5.5rem] gap-2">
        <FormField v-slot="{ id }" label="Manual price" required :error="errors.price">
          <input :id="id" v-model="lockForm.price" type="number" min="0" step="any" class="input" required data-testid="lock-price" />
        </FormField>
        <FormField v-slot="{ id }" label="Currency" :error="errors.price_currency">
          <input :id="id" v-model="lockForm.currency" class="input uppercase" maxlength="3" />
        </FormField>
      </div>
      <FormField v-slot="{ id }" label="Compare-at price" :error="errors.compare_at_price">
        <input :id="id" v-model="lockForm.compare_at" type="number" min="0" step="any" class="input" />
      </FormField>
      <div class="flex gap-2">
        <UiButton type="submit" size="sm" variant="primary" :loading="busy === 'lock'">Lock price</UiButton>
        <UiButton size="sm" variant="ghost" @click="locking = false">Cancel</UiButton>
      </div>
    </form>

    <div v-if="canEdit && !locking" class="mt-3 flex flex-wrap gap-2">
      <UiButton v-if="!product.price_locked" size="sm" @click="startLock"><Lock class="h-4 w-4" aria-hidden="true" /> Set a manual price</UiButton>
      <UiButton v-else size="sm" :loading="busy === 'unlock'" @click="unlock"><LockOpen class="h-4 w-4" aria-hidden="true" /> Unlock price</UiButton>
      <UiButton size="sm" :loading="busy === 'recalc'" @click="recalculate"><RefreshCw class="h-4 w-4" aria-hidden="true" /> Recalculate</UiButton>
    </div>
  </div>
</template>
