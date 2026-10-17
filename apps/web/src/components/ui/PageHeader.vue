<script setup lang="ts">
defineProps<{ title: string; subtitle?: string; back?: string }>()
</script>

<template>
  <div class="mb-5 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
    <div class="min-w-0">
      <RouterLink v-if="back" :to="back" class="mb-1 inline-block text-xs text-slate-500 hover:text-slate-800">← Back</RouterLink>
      <h1 class="truncate text-xl font-semibold text-slate-900">{{ title }}</h1>
      <p v-if="subtitle" class="mt-0.5 text-sm text-slate-500">{{ subtitle }}</p>
      <slot name="meta" />
    </div>
    <div class="flex flex-wrap items-center gap-2"><slot name="actions" /></div>
  </div>
</template>
