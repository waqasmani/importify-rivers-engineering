<script setup lang="ts">
import { AlertTriangle, Inbox } from 'lucide-vue-next'
import UiButton from './UiButton.vue'

defineProps<{ kind: 'loading' | 'empty' | 'error'; title?: string; message?: string }>()
defineEmits<{ retry: [] }>()
</script>

<template>
  <div class="flex flex-col items-center justify-center gap-2 px-4 py-12 text-center" :role="kind === 'error' ? 'alert' : 'status'">
    <template v-if="kind === 'loading'">
      <svg class="h-6 w-6 animate-spin text-brand-600" viewBox="0 0 24 24" fill="none" aria-hidden="true">
        <circle cx="12" cy="12" r="10" stroke="currentColor" stroke-opacity="0.25" stroke-width="4" />
        <path d="M22 12a10 10 0 0 0-10-10" stroke="currentColor" stroke-width="4" stroke-linecap="round" />
      </svg>
      <p class="text-sm text-slate-500">{{ message ?? 'Loading…' }}</p>
    </template>
    <template v-else-if="kind === 'empty'">
      <Inbox class="h-8 w-8 text-slate-300" aria-hidden="true" />
      <p class="text-sm font-medium text-slate-700">{{ title ?? 'Nothing here yet' }}</p>
      <p v-if="message" class="max-w-md text-sm text-slate-500">{{ message }}</p>
      <slot />
    </template>
    <template v-else>
      <AlertTriangle class="h-8 w-8 text-red-400" aria-hidden="true" />
      <p class="text-sm font-medium text-slate-700">{{ title ?? 'Could not load this page' }}</p>
      <p v-if="message" class="max-w-md text-sm text-slate-500">{{ message }}</p>
      <UiButton size="sm" @click="$emit('retry')">Try again</UiButton>
    </template>
  </div>
</template>
