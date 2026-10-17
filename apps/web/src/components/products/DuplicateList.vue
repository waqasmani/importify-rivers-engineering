<script setup lang="ts">
// Duplicate candidates with their match score and the reasons REA found them. Optionally lets the
// user pick one as the product to update.
import { useId } from 'vue'
import UiBadge from '@/components/ui/UiBadge.vue'
import { scoreLabel, type DupSummary } from './product-utils'

withDefaults(defineProps<{ candidates: DupSummary[]; selectable?: boolean; legend?: string }>(), { selectable: false, legend: 'Possible duplicates' })
const selected = defineModel<string | null>({ default: null })
const name = useId()

function tone(score: number): 'red' | 'amber' | 'slate' {
  if (score >= 0.9) return 'red'
  if (score >= 0.7) return 'amber'
  return 'slate'
}
</script>

<template>
  <fieldset>
    <legend class="label">{{ legend }}</legend>
    <p v-if="candidates.length === 0" class="text-sm text-slate-500">No similar products were found.</p>
    <ul v-else class="divide-y divide-slate-100 rounded-md border border-slate-200 bg-white" data-testid="duplicate-list">
      <li v-for="c in candidates" :key="c.id" class="flex items-start gap-3 px-3 py-2">
        <input
          v-if="selectable"
          :id="`${name}-${c.id}`"
          v-model="selected"
          type="radio"
          :name="name"
          :value="c.id"
          class="mt-1"
          :aria-label="`Use ${c.label}`"
        />
        <div class="min-w-0 flex-1">
          <div class="flex flex-wrap items-center gap-2">
            <RouterLink :to="`/products/${c.id}`" class="truncate text-sm font-medium text-brand-700 hover:underline">{{ c.label }}</RouterLink>
            <UiBadge :tone="tone(c.score)">{{ scoreLabel(c.score) }} match</UiBadge>
          </div>
          <p v-if="c.reasons.length" class="mt-0.5 text-xs text-slate-500">{{ c.reasons.join(' · ') }}</p>
        </div>
      </li>
    </ul>
  </fieldset>
</template>
