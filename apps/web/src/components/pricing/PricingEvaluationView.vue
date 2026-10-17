<script setup lang="ts">
// One pricing evaluation: the rule that matched, every calculation step with its running amount,
// and the resulting selling price, compare-at price and margin. Used by the rule preview, the
// product price breakdown and the import review screens.
import { computed } from 'vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import { formatMoney } from '@/lib/format'
import { PRICING_STATUS, formatAmount, statusTone, type Evaluation } from './pricing-utils'

const props = withDefaults(defineProps<{ evaluation: Evaluation; compact?: boolean; showSteps?: boolean }>(), { compact: false, showSteps: true })

const priced = computed(() => props.evaluation.status === 'priced')
const steps = computed(() => props.evaluation.steps.filter((s) => s.kind !== 'price'))
</script>

<template>
  <div class="space-y-2" data-testid="pricing-evaluation">
    <div class="flex flex-wrap items-center gap-2">
      <UiBadge :tone="statusTone(evaluation.status)">{{ PRICING_STATUS[evaluation.status] ?? evaluation.status }}</UiBadge>
      <span v-if="evaluation.rule" class="text-sm text-slate-700">Rule: <span class="font-medium" data-testid="evaluation-rule">{{ evaluation.rule.name }}</span></span>
    </div>
    <p v-if="!priced && evaluation.message" class="text-sm text-amber-800" role="status">{{ evaluation.message }}</p>

    <ol v-if="showSteps && priced && steps.length" class="divide-y divide-slate-100 rounded-md border border-slate-200 bg-white text-sm" aria-label="Calculation steps">
      <li v-for="(s, i) in steps" :key="i" class="flex items-baseline justify-between gap-3 px-3 py-1.5" :data-step="s.kind">
        <span class="min-w-0 text-slate-600">{{ s.label }}</span>
        <span class="shrink-0 font-mono text-slate-900 tabular-nums">{{ formatAmount(s.amount, evaluation.currency) }}</span>
      </li>
    </ol>

    <dl v-if="priced" class="grid gap-2 text-sm" :class="compact ? 'grid-cols-2' : 'grid-cols-2 sm:grid-cols-4'">
      <div>
        <dt class="text-xs font-medium text-slate-500">Selling price</dt>
        <dd class="text-base font-semibold text-slate-900" data-testid="evaluation-price">{{ formatMoney(evaluation.price, evaluation.currency) }}</dd>
      </div>
      <div>
        <dt class="text-xs font-medium text-slate-500">Compare-at</dt>
        <dd class="text-slate-900" data-testid="evaluation-compare-at">{{ evaluation.compare_at_price ? formatMoney(evaluation.compare_at_price, evaluation.currency) : '—' }}</dd>
      </div>
      <div>
        <dt class="text-xs font-medium text-slate-500">Cost</dt>
        <dd class="text-slate-900">
          {{ formatAmount(evaluation.base_cost, evaluation.currency) }}
          <span v-if="evaluation.rate" class="block text-xs text-slate-500">from {{ formatAmount(evaluation.cost, evaluation.cost_currency) }}</span>
        </dd>
      </div>
      <div>
        <dt class="text-xs font-medium text-slate-500">Margin</dt>
        <dd class="text-slate-900" data-testid="evaluation-margin">
          {{ formatMoney(evaluation.margin_amount, evaluation.currency) }}<template v-if="evaluation.margin_percent !== null"> · {{ Number(evaluation.margin_percent).toFixed(2) }}%</template>
        </dd>
      </div>
    </dl>
    <ul v-if="evaluation.warnings.length" class="list-disc pl-5 text-xs text-amber-800">
      <li v-for="w in evaluation.warnings" :key="w">{{ w }}</li>
    </ul>
  </div>
</template>
