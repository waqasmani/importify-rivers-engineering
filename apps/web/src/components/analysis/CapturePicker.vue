<script setup lang="ts">
// Picks one page captured with the browser extension (GET /captures).
import { computed, ref, useId, watch } from 'vue'
import { refDebounced } from '@vueuse/core'
import { useQuery } from '@tanstack/vue-query'
import { api, errorMessage, type Schemas } from '@/api/client'
import UiButton from '@/components/ui/UiButton.vue'
import { formatDateTime } from '@/lib/format'

const model = defineModel<string | null>({ default: null })
const props = defineProps<{ label?: string; error?: string }>()
const emit = defineEmits<{ select: [label: string] }>()
const uid = useId()

const text = ref('')
const debounced = refDebounced(text, 250)
const browsing = ref(!model.value)
watch(model, (v) => {
  if (!v) browsing.value = true
})

const list = useQuery({
  queryKey: ['capture-picker', debounced],
  queryFn: ({ signal }) => api.get<Schemas['ListCapturedPagesRowPage']>('/captures', { q: debounced.value.trim(), limit: 10 }, signal),
  enabled: browsing,
})
const items = computed(() => list.data.value?.items ?? [])

// Resolve the label of a preselected capture (for example from ?capture=<id>).
const detail = useQuery({
  queryKey: ['capture-picker-detail', model],
  queryFn: ({ signal }) => api.get<Schemas['CaptureDetailDoc']>(`/captures/${model.value}`, undefined, signal),
  enabled: computed(() => !!model.value && !props.label),
})
const selectedLabel = computed(() => props.label || detail.data.value?.capture.title || detail.data.value?.capture.url || '')
watch(selectedLabel, (l) => {
  if (l && !props.label) emit('select', l)
})

function choose(c: Schemas['ListCapturedPagesRow']) {
  model.value = c.id
  browsing.value = false
  emit('select', c.title || c.url)
}
</script>

<template>
  <div>
    <div v-if="model && !browsing" class="flex items-center justify-between gap-2 rounded-md border border-slate-300 bg-white px-3 py-2 text-sm">
      <span class="min-w-0 truncate" data-testid="capture-selected">{{ selectedLabel || (detail.isError.value ? 'Capture not found' : 'Loading…') }}</span>
      <UiButton size="sm" variant="ghost" @click="browsing = true">Change</UiButton>
    </div>
    <template v-else>
      <label :for="`${uid}-q`" class="sr-only">Search captured pages</label>
      <input :id="`${uid}-q`" v-model="text" type="search" class="input" placeholder="Search captured pages by title or URL" autocomplete="off" />
      <div class="mt-2 max-h-64 overflow-y-auto rounded-md border border-slate-200 bg-white" role="listbox" :aria-label="'Captured pages'">
        <p v-if="list.isLoading.value" class="px-3 py-2 text-sm text-slate-500" role="status">Loading captured pages…</p>
        <p v-else-if="list.error.value" class="px-3 py-2 text-sm text-red-600" role="alert">{{ errorMessage(list.error.value) }}</p>
        <p v-else-if="items.length === 0" class="px-3 py-2 text-sm text-slate-500">
          No captured pages{{ debounced ? ' match' : ' yet' }}. Capture pages with the REA browser extension first.
        </p>
        <button
          v-for="c in items"
          :key="c.id"
          type="button"
          role="option"
          :aria-selected="model === c.id"
          class="block w-full border-b border-slate-100 px-3 py-2 text-left text-sm last:border-0 hover:bg-slate-50"
          :class="model === c.id ? 'bg-brand-50' : ''"
          @click="choose(c)"
        >
          <span class="block truncate font-medium text-slate-900">{{ c.title || c.url }}</span>
          <span class="block truncate text-xs text-slate-500">{{ c.domain }} · {{ c.page_type.replace(/_/g, ' ') }} · {{ formatDateTime(c.captured_at) }}{{ c.has_snapshot ? ' · page snapshot' : '' }}</span>
        </button>
      </div>
      <UiButton v-if="model" size="sm" variant="ghost" class="mt-1" @click="browsing = false">Keep current selection</UiButton>
    </template>
    <p v-if="error" class="mt-1 text-xs text-red-600" role="alert">{{ error }}</p>
  </div>
</template>
