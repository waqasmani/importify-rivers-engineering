<script setup lang="ts">
// Collapsible, pretty-printed JSON. Rendered as text, never as HTML.
import { computed } from 'vue'

const props = withDefaults(defineProps<{ value: unknown; summary: string; open?: boolean }>(), { open: false })
const text = computed(() => {
  try {
    return JSON.stringify(props.value ?? null, null, 2)
  } catch {
    return String(props.value)
  }
})
const empty = computed(() => {
  const v = props.value
  if (v === null || v === undefined) return true
  if (Array.isArray(v)) return v.length === 0
  if (typeof v === 'object') return Object.keys(v as object).length === 0
  return false
})
</script>

<template>
  <details class="rounded-md border border-slate-200 bg-white" :open="open">
    <summary class="cursor-pointer px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50">
      {{ summary }} <span v-if="empty" class="font-normal text-slate-400">(empty)</span>
    </summary>
    <pre class="max-h-[32rem] overflow-auto border-t border-slate-200 bg-slate-50 p-3 font-mono text-xs break-words whitespace-pre-wrap text-slate-800">{{ text }}</pre>
  </details>
</template>
