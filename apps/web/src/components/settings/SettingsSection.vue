<script setup lang="ts">
// Card used by settings pages: a heading, an explanation and the section's content.
import { useId } from 'vue'

defineProps<{ title: string; description?: string }>()
const id = useId()
</script>

<template>
  <section class="card min-w-0" :aria-labelledby="id">
    <header class="flex flex-col gap-2 border-b border-slate-100 px-4 py-3 sm:flex-row sm:items-start sm:justify-between">
      <div class="min-w-0">
        <h2 :id="id" class="text-sm font-semibold text-slate-900">{{ title }}</h2>
        <p v-if="description" class="mt-0.5 text-sm text-slate-500">{{ description }}</p>
      </div>
      <div v-if="$slots.actions" class="flex shrink-0 flex-wrap gap-2"><slot name="actions" /></div>
    </header>
    <div class="p-4"><slot /></div>
  </section>
</template>
