<script setup lang="ts">
import { computed } from 'vue'
import UiButton from './UiButton.vue'

const props = defineProps<{ total: number; limit: number; offset: number }>()
const emit = defineEmits<{ 'update:offset': [value: number] }>()
const from = computed(() => (props.total === 0 ? 0 : props.offset + 1))
const to = computed(() => Math.min(props.offset + props.limit, props.total))
</script>

<template>
  <nav class="mt-3 flex items-center justify-between text-sm text-slate-600" aria-label="Pagination">
    <p>{{ from }}–{{ to }} of {{ total }}</p>
    <div class="flex gap-2">
      <UiButton size="sm" :disabled="offset === 0" @click="emit('update:offset', Math.max(0, offset - limit))">Previous</UiButton>
      <UiButton size="sm" :disabled="offset + limit >= total" @click="emit('update:offset', offset + limit)">Next</UiButton>
    </div>
  </nav>
</template>
