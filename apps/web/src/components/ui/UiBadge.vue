<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{ value?: string | null; color?: string; tone?: 'green' | 'red' | 'amber' | 'blue' | 'slate' | 'violet' }>()

const autoTone: Record<string, 'green' | 'red' | 'amber' | 'blue' | 'slate' | 'violet'> = {
  active: 'green', approved: 'green', won: 'green', succeeded: 'green', done: 'green', imported: 'green', verified: 'green', in_stock: 'green', ready: 'green',
  pending: 'amber', needs_changes: 'amber', queued: 'amber', running: 'blue', in_progress: 'blue', open: 'blue', draft: 'slate', duplicate: 'amber',
  inferred: 'violet', unknown: 'slate', rejected: 'red', failed: 'red', lost: 'red', archived: 'slate', out_of_stock: 'red', cancelled: 'slate',
  updated: 'blue', urgent: 'red', high: 'amber',
}
const tones = {
  green: 'bg-green-50 text-green-700 ring-green-600/20', red: 'bg-red-50 text-red-700 ring-red-600/20', amber: 'bg-amber-50 text-amber-800 ring-amber-600/20',
  blue: 'bg-sky-50 text-sky-700 ring-sky-600/20', slate: 'bg-slate-100 text-slate-700 ring-slate-500/20', violet: 'bg-violet-50 text-violet-700 ring-violet-600/20',
}
const cls = computed(() => tones[props.tone ?? autoTone[props.value ?? ''] ?? 'slate'])
</script>

<template>
  <span class="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium whitespace-nowrap ring-1 ring-inset" :class="color ? '' : cls" :style="color ? { backgroundColor: `${color}1a`, color } : undefined">
    <slot>{{ (value ?? '').replace(/_/g, ' ') }}</slot>
  </span>
</template>
