<script setup lang="ts">
import { TriangleAlert } from 'lucide-vue-next'
import type { UiError } from '../../shared/messages'

defineProps<{ error: UiError | null | undefined }>()
</script>

<template>
  <div v-if="error" role="alert" class="flex gap-2 rounded-md border border-red-200 bg-red-50 p-2 text-[13px] text-red-800">
    <TriangleAlert class="mt-0.5 size-4 shrink-0" aria-hidden="true" />
    <div class="min-w-0 space-y-1">
      <p class="break-words">{{ error.message }}</p>
      <ul v-if="error.fields" class="list-disc pl-4 text-xs">
        <li v-for="(msg, field) in error.fields" :key="field"><span class="font-medium">{{ field }}</span> {{ msg }}</li>
      </ul>
    </div>
  </div>
</template>
