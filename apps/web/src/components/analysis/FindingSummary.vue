<script setup lang="ts">
// Finding counts by category and classification. Each count links to the filtered findings list.
import { computed } from 'vue'
import type { Schemas } from '@/api/client'
import ClassificationBadge from './ClassificationBadge.vue'
import { CLASSIFICATIONS } from './options'

const props = defineProps<{ rows: Schemas['FindingCategoryCountsRow'][] }>()
const emit = defineEmits<{ open: [filters: { category?: string; classification?: string }] }>()

const matrix = computed(() => {
  const by = new Map<string, Record<string, number>>()
  for (const r of props.rows) {
    const row = by.get(r.category) ?? { verified: 0, inferred: 0, unknown: 0 }
    row[r.classification] = (row[r.classification] ?? 0) + r.finding_count
    by.set(r.category, row)
  }
  return [...by.entries()].sort((a, b) => a[0].localeCompare(b[0])).map(([category, c]) => ({ category, c, total: CLASSIFICATIONS.reduce((n, k) => n + (c[k] ?? 0), 0) }))
})
const totals = computed(() => {
  const t: Record<string, number> = { verified: 0, inferred: 0, unknown: 0 }
  for (const r of props.rows) t[r.classification] = (t[r.classification] ?? 0) + r.finding_count
  return t
})
const grand = computed(() => CLASSIFICATIONS.reduce((n, k) => n + (totals.value[k] ?? 0), 0))
const barClass: Record<string, string> = { verified: 'bg-emerald-500', inferred: 'bg-amber-400 bg-[repeating-linear-gradient(45deg,transparent,transparent_4px,rgba(255,255,255,.45)_4px,rgba(255,255,255,.45)_8px)]', unknown: 'bg-slate-300' }
</script>

<template>
  <div class="space-y-4" data-testid="finding-summary">
    <div v-if="grand > 0">
      <div class="flex h-3 overflow-hidden rounded-full bg-slate-100" role="img" :aria-label="CLASSIFICATIONS.map((k) => `${totals[k]} ${k}`).join(', ')">
        <span v-for="k in CLASSIFICATIONS" :key="k" :class="barClass[k]" :style="{ width: `${(100 * (totals[k] ?? 0)) / grand}%` }" />
      </div>
      <ul class="mt-2 flex flex-wrap gap-3 text-sm">
        <li v-for="k in CLASSIFICATIONS" :key="k">
          <button type="button" class="flex items-center gap-1.5 rounded hover:underline" @click="emit('open', { classification: k })"><ClassificationBadge :value="k" /> <span class="font-semibold tabular-nums">{{ totals[k] }}</span></button>
        </li>
      </ul>
    </div>
    <div v-if="matrix.length" class="overflow-x-auto">
      <table class="min-w-full text-sm">
        <thead>
          <tr class="border-b border-slate-200">
            <th scope="col" class="px-2 py-2 text-xs font-semibold tracking-wide text-slate-500 uppercase sm:px-3 text-left">Category</th>
            <th v-for="k in CLASSIFICATIONS" :key="k" scope="col" class="px-2 py-2 text-xs font-semibold tracking-wide text-slate-500 uppercase sm:px-3 text-right">{{ k }}</th>
            <th scope="col" class="px-2 py-2 text-xs font-semibold tracking-wide text-slate-500 uppercase sm:px-3 text-right">Total</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-slate-100">
          <tr v-for="m in matrix" :key="m.category">
            <th scope="row" class="px-2 py-2 text-sm text-slate-700 sm:px-3 text-left font-medium">
              <button type="button" class="hover:underline" @click="emit('open', { category: m.category })">{{ m.category.replace(/_/g, ' ') }}</button>
            </th>
            <td v-for="k in CLASSIFICATIONS" :key="k" class="px-2 py-2 text-sm text-slate-700 sm:px-3 text-right tabular-nums">
              <button v-if="m.c[k]" type="button" class="text-brand-700 hover:underline" :aria-label="`${m.c[k]} ${k} ${m.category} findings`" @click="emit('open', { category: m.category, classification: k })">{{ m.c[k] }}</button>
              <span v-else class="text-slate-300">0</span>
            </td>
            <td class="px-2 py-2 text-sm text-slate-700 sm:px-3 text-right font-medium tabular-nums">{{ m.total }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
