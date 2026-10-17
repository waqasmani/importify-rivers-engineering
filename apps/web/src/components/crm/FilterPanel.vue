<script setup lang="ts">
// Layout for list filters: search and saved filters on top, the other filters below. On small
// screens the extra filters collapse behind a toggle.
import { ref } from 'vue'
import { SlidersHorizontal } from 'lucide-vue-next'
import UiButton from '@/components/ui/UiButton.vue'

defineProps<{ active: boolean }>()
const emit = defineEmits<{ clear: [] }>()
const shown = ref(false)
</script>

<template>
  <div class="card mb-3 p-3">
    <div class="flex flex-wrap items-center gap-2">
      <slot name="search" />
      <UiButton class="sm:hidden" :aria-expanded="shown" aria-controls="list-filters" @click="shown = !shown">
        <SlidersHorizontal class="h-4 w-4" /> Filters<span v-if="active" class="ml-1 h-2 w-2 rounded-full bg-brand-600" aria-label="active" />
      </UiButton>
      <div class="ml-auto flex items-center gap-2">
        <UiButton v-if="active" variant="ghost" size="sm" @click="emit('clear')">Clear filters</UiButton>
        <slot name="saved" />
      </div>
    </div>
    <div id="list-filters" class="mt-3 grid-cols-1 gap-3 sm:grid sm:grid-cols-3 lg:grid-cols-6" :class="shown ? 'grid' : 'hidden'">
      <slot />
    </div>
  </div>
</template>
