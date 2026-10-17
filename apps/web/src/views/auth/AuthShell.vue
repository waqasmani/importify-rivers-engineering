<script setup lang="ts">
defineProps<{ title: string; subtitle?: string; wide?: boolean }>()
</script>

<template>
  <div class="flex min-h-screen items-center justify-center bg-slate-50 px-4 py-10">
    <div class="w-full" :class="wide ? 'max-w-xl' : 'max-w-sm'">
      <div class="mb-6 flex flex-col items-center gap-2 text-center">
        <img src="/favicon.svg" alt="" class="h-10 w-10" />
        <h1 class="text-xl font-semibold text-slate-900">{{ title }}</h1>
        <p v-if="subtitle" class="text-sm text-slate-500">{{ subtitle }}</p>
      </div>
      <div class="card p-6"><slot /></div>
      <p class="mt-4 text-center text-xs text-slate-400">REA Importify · self-hosted · no telemetry</p>
    </div>
  </div>
</template>
