<script setup lang="ts">
// Search input for list pages. Commits after a short pause or immediately on Enter.
import { ref, useId, watch } from 'vue'
import { watchDebounced } from '@vueuse/core'
import { Search } from 'lucide-vue-next'

const props = withDefaults(defineProps<{ modelValue: string; label: string; placeholder?: string }>(), { placeholder: 'Search…' })
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const id = useId()
const text = ref(props.modelValue)
watch(
  () => props.modelValue,
  (v) => {
    if (v !== text.value.trim()) text.value = v
  },
)
function commit() {
  const v = text.value.trim()
  if (v !== props.modelValue) emit('update:modelValue', v)
}
watchDebounced(text, commit, { debounce: 350 })
</script>

<template>
  <form role="search" class="relative w-full min-w-0 sm:w-auto sm:max-w-xs sm:flex-1" @submit.prevent="commit">
    <label :for="id" class="sr-only">{{ label }}</label>
    <Search class="pointer-events-none absolute top-2.5 left-2.5 h-4 w-4 text-slate-400" aria-hidden="true" />
    <input :id="id" v-model="text" type="search" class="input pl-8" :placeholder="placeholder" data-testid="list-search" />
  </form>
</template>
