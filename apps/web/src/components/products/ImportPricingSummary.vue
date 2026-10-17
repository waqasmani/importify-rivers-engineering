<script setup lang="ts">
// The selling price the current pricing rules give a capture under review: source price → cost →
// rule → selling price for the product and each variant. Nothing is stored by showing it.
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import type { Schemas } from '@/api/client'
import UiBadge from '@/components/ui/UiBadge.vue'
import PricingEvaluationView from '@/components/pricing/PricingEvaluationView.vue'
import { useAuth } from '@/stores/auth'
import { formatMoney } from '@/lib/format'
import { PRICING_STATUS, statusTone } from '@/components/pricing/pricing-utils'

const props = withDefaults(defineProps<{ pricing: Schemas['ImportPricing']; title?: string; note?: string }>(), {
  title: 'Selling price',
  note: 'With the current pricing rules and exchange rates. An existing product keeps a locked manual price or a cost entered by hand.',
})
const auth = useAuth()
const variants = computed(() => props.pricing.variants)
</script>

<template>
  <section class="space-y-3 rounded-md border border-slate-200 bg-white p-3" aria-label="Computed selling price" data-testid="import-pricing">
    <div class="flex flex-wrap items-baseline justify-between gap-2">
      <h3 class="text-sm font-semibold text-slate-800">{{ title }}</h3>
      <RouterLink v-if="auth.can('settings.manage')" to="/settings/pricing" class="text-xs text-brand-700 hover:underline">Pricing rules</RouterLink>
    </div>
    <p class="text-xs text-slate-500">{{ note }}</p>
    <PricingEvaluationView :evaluation="pricing.product" />
    <div v-if="variants.length" class="overflow-x-auto">
      <table class="min-w-full divide-y divide-slate-200 text-sm">
        <thead>
          <tr>
            <th scope="col" class="table-th">Variant</th>
            <th scope="col" class="table-th text-right">Cost</th>
            <th scope="col" class="table-th text-right">Selling price</th>
            <th scope="col" class="table-th">Rule</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-slate-100">
          <tr v-for="v in variants" :key="v.index" :data-testid="`import-variant-price-${v.index}`">
            <td class="table-td">{{ v.title || `Variant ${v.index + 1}` }}</td>
            <td class="table-td text-right whitespace-nowrap">{{ formatMoney(v.evaluation.base_cost, v.evaluation.currency) }}</td>
            <td class="table-td text-right font-medium whitespace-nowrap">
              <template v-if="v.evaluation.status === 'priced'">{{ formatMoney(v.evaluation.price, v.evaluation.currency) }}</template>
              <UiBadge v-else :tone="statusTone(v.evaluation.status)">{{ PRICING_STATUS[v.evaluation.status] ?? v.evaluation.status }}</UiBadge>
            </td>
            <td class="table-td text-slate-600">{{ v.evaluation.rule?.name ?? '—' }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>
