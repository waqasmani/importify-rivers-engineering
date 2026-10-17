<script setup lang="ts">
defineProps<{ title: string; subtitle?: string }>()
</script>

<template>
  <section class="card space-y-3" :aria-label="title">
    <header class="flex items-start justify-between gap-2">
      <div class="min-w-0">
        <h2 class="text-sm font-semibold text-slate-900">{{ title }}</h2>
        <p v-if="subtitle" class="muted mt-0.5">{{ subtitle }}</p>
      </div>
      <slot name="actions" />
    </header>
    <slot />
  </section>
</template>
