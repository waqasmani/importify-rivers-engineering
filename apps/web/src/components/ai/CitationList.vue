<script setup lang="ts">
// Evidence references like ["E1", "E3"] cited by an AI output, shown as small chips.
defineProps<{ refs?: unknown }>()
</script>

<template>
  <span v-if="Array.isArray(refs) && refs.length" class="ml-1 inline-flex flex-wrap gap-0.5 align-middle">
    <span v-for="r in refs as string[]" :key="r" class="rounded bg-slate-100 px-1 font-mono text-[10px] text-slate-600" :title="`Evidence ${r}`">{{ r }}</span>
  </span>
  <span v-else-if="refs !== undefined" class="ml-1 rounded bg-amber-50 px-1 text-[10px] text-amber-700" title="No evidence was cited for this item">uncited</span>
</template>
