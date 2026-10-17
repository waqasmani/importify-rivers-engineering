<script setup lang="ts">
import { ExternalLink } from 'lucide-vue-next'

defineProps<{ href: string }>()
</script>

<template>
  <a :href="href" target="_blank" rel="noopener noreferrer" class="inline-flex items-center gap-1 font-medium underline-offset-2 hover:underline">
    <slot />
    <ExternalLink class="size-3.5 shrink-0" aria-hidden="true" />
  </a>
</template>
