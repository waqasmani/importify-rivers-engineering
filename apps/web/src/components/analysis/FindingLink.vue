<script setup lang="ts">
// Compact link to a finding with its classification, used inside source viewers.
import ClassificationBadge from './ClassificationBadge.vue'

defineProps<{ id: string; classification: string; label?: string }>()
</script>

<template>
  <RouterLink :to="`/analysis/findings/${id}`" class="inline-flex items-center gap-1 text-xs text-brand-700 hover:underline" :aria-label="`Open finding${label ? `: ${label}` : ''}`">
    <ClassificationBadge :value="classification" />
  </RouterLink>
</template>
