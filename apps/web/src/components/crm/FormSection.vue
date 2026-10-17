<script setup lang="ts">
defineProps<{ title: string }>()
</script>

<template>
  <fieldset class="space-y-4">
    <legend class="mb-3 text-xs font-semibold tracking-wide text-slate-500 uppercase">{{ title }}</legend>
    <div class="grid gap-4 sm:grid-cols-2"><slot /></div>
  </fieldset>
</template>
