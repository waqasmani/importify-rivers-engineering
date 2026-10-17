<script setup lang="ts">
// Compact tag badges for table cells.
import UiBadge from '@/components/ui/UiBadge.vue'
defineProps<{ tags?: { id: string; name: string; color: string }[] }>()
</script>

<template>
  <span v-if="tags?.length" class="flex flex-wrap gap-1">
    <UiBadge v-for="t in tags.slice(0, 3)" :key="t.id" :color="t.color || undefined">{{ t.name }}</UiBadge>
    <span v-if="tags.length > 3" class="text-xs text-slate-500">+{{ tags.length - 3 }}</span>
  </span>
  <span v-else class="text-slate-400">—</span>
</template>
