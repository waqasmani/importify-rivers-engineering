<script setup lang="ts">
// Verified evidence, inferences and unknowns must never look alike: each has its own colour,
// icon, border style and text, so the difference survives without colour too.
import { computed } from 'vue'
import { CircleCheck, CircleHelp, Lightbulb } from 'lucide-vue-next'
import { CLASSIFICATION_INFO, type Classification } from './options'

const props = withDefaults(defineProps<{ value: string; size?: 'sm' | 'md' }>(), { size: 'sm' })
const c = computed<Classification>(() => (props.value === 'verified' || props.value === 'inferred' ? props.value : 'unknown'))
const styles: Record<Classification, string> = {
  verified: 'border-solid border-emerald-600/40 bg-emerald-50 text-emerald-800',
  inferred: 'border-dashed border-amber-600/60 bg-amber-50 text-amber-900',
  unknown: 'border-dotted border-slate-500/60 bg-slate-100 text-slate-700',
}
const icons = { verified: CircleCheck, inferred: Lightbulb, unknown: CircleHelp }
</script>

<template>
  <span
    class="inline-flex items-center gap-1 rounded-full border font-medium whitespace-nowrap"
    :class="[styles[c], size === 'md' ? 'px-2.5 py-1 text-sm' : 'px-2 py-0.5 text-xs']"
    :title="CLASSIFICATION_INFO[c].description"
    :data-classification="c"
    data-testid="classification"
  >
    <component :is="icons[c]" :class="size === 'md' ? 'h-4 w-4' : 'h-3 w-3'" aria-hidden="true" />
    {{ CLASSIFICATION_INFO[c].label }}
  </span>
</template>
