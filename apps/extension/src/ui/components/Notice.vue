<script setup lang="ts">
import { CircleCheck, Info, TriangleAlert } from 'lucide-vue-next'

const props = withDefaults(defineProps<{ kind?: 'info' | 'warn' | 'success' }>(), { kind: 'info' })
const styles = {
  info: 'border-sky-200 bg-sky-50 text-sky-900',
  warn: 'border-amber-200 bg-amber-50 text-amber-900',
  success: 'border-emerald-200 bg-emerald-50 text-emerald-900',
}
const icon = { info: Info, warn: TriangleAlert, success: CircleCheck }[props.kind]
</script>

<template>
  <div class="flex gap-2 rounded-md border p-2 text-[13px]" :class="styles[kind]" :role="kind === 'warn' ? 'status' : undefined">
    <component :is="icon" class="mt-0.5 size-4 shrink-0" aria-hidden="true" />
    <div class="min-w-0 space-y-1 break-words"><slot /></div>
  </div>
</template>
