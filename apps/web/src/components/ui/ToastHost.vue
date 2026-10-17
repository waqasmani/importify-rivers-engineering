<script setup lang="ts">
import { CheckCircle2, Info, X, XCircle } from 'lucide-vue-next'
import { useToast } from '@/composables/useToast'

const { toasts, dismiss } = useToast()
</script>

<template>
  <div class="pointer-events-none fixed inset-x-0 bottom-4 z-50 flex flex-col items-center gap-2 px-4 sm:items-end" aria-live="polite">
    <div
      v-for="t in toasts"
      :key="t.id"
      class="pointer-events-auto flex w-full max-w-sm items-start gap-2 rounded-lg border bg-white px-3 py-2 text-sm shadow-lg"
      :class="{ 'border-green-200': t.kind === 'success', 'border-red-200': t.kind === 'error', 'border-slate-200': t.kind === 'info' }"
      :role="t.kind === 'error' ? 'alert' : 'status'"
    >
      <CheckCircle2 v-if="t.kind === 'success'" class="mt-0.5 h-4 w-4 shrink-0 text-green-600" />
      <XCircle v-else-if="t.kind === 'error'" class="mt-0.5 h-4 w-4 shrink-0 text-red-600" />
      <Info v-else class="mt-0.5 h-4 w-4 shrink-0 text-slate-500" />
      <p class="flex-1 text-slate-700">{{ t.message }}</p>
      <button type="button" class="text-slate-400 hover:text-slate-600" aria-label="Dismiss" @click="dismiss(t.id)"><X class="h-4 w-4" /></button>
    </div>
  </div>
</template>
