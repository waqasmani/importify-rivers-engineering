<script setup lang="ts">
// Confidence 0–1 as a labelled meter. Width and text carry the value; colour is secondary.
import { computed } from 'vue'

const props = withDefaults(defineProps<{ value?: string | number | null; showLabel?: boolean; wide?: boolean }>(), { value: null, showLabel: true, wide: false })
const pct = computed(() => {
  const n = Number(props.value)
  return Number.isFinite(n) ? Math.max(0, Math.min(100, Math.round(n * 100))) : 0
})
const tone = computed(() => (pct.value >= 80 ? 'bg-emerald-500' : pct.value >= 50 ? 'bg-amber-500' : 'bg-slate-400'))
</script>

<template>
  <span class="inline-flex items-center gap-2" data-testid="confidence">
    <span
      class="block h-1.5 overflow-hidden rounded-full bg-slate-200"
      :class="wide ? 'w-40' : 'w-14'"
      role="meter"
      aria-label="Confidence"
      :aria-valuenow="pct"
      aria-valuemin="0"
      aria-valuemax="100"
      :aria-valuetext="`${pct}%`"
    >
      <span class="block h-full rounded-full" :class="tone" :style="{ width: `${pct}%` }" />
    </span>
    <span v-if="showLabel" class="text-xs text-slate-600 tabular-nums">{{ pct }}%</span>
  </span>
</template>
