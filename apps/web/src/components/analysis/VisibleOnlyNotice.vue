<script setup lang="ts">
// Explains what visible websites and screenshots can and cannot support.
import { Eye } from 'lucide-vue-next'
import { VISIBLE_ONLY_NOTICE } from './options'

defineProps<{ compact?: boolean }>()
</script>

<template>
  <p class="flex items-start gap-2 rounded-md border border-sky-200 bg-sky-50 px-3 py-2 text-sky-900" :class="compact ? 'text-xs' : 'text-sm'" role="note" data-testid="visible-only-notice">
    <Eye class="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
    <span><strong class="font-semibold">Visible behaviour only.</strong> {{ VISIBLE_ONLY_NOTICE }}</span>
  </p>
</template>
