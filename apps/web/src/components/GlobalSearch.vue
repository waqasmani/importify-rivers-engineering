<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { onKeyStroke, refDebounced } from '@vueuse/core'
import { useQuery } from '@tanstack/vue-query'
import { Search } from 'lucide-vue-next'
import { api, type Schemas } from '@/api/client'
import { entityLabel } from '@/lib/entities'

const router = useRouter()
const input = ref<HTMLInputElement | null>(null)
const q = ref('')
const open = ref(false)
const active = ref(0)
const debounced = refDebounced(q, 200)

const enabled = computed(() => debounced.value.trim().length >= 2)
const { data, isFetching } = useQuery({
  queryKey: ['search', debounced],
  queryFn: ({ signal }) => api.get<Schemas['SearchDoc']>('/search', { q: debounced.value.trim() }, signal),
  enabled,
})
const hits = computed(() => (enabled.value ? (data.value?.items ?? []) : []))
watch(hits, () => (active.value = 0))

const linkFor: Record<string, (id: string) => string> = {
  company: (id) => `/companies/${id}`,
  contact: (id) => `/contacts/${id}`,
  supplier: (id) => `/suppliers/${id}`,
  lead: (id) => `/leads/${id}`,
  opportunity: (id) => `/opportunities/${id}`,
  task: (id) => `/tasks/${id}`,
  product: (id) => `/products/${id}`,
  captured_page: (id) => `/captures/${id}`,
  analysis_project: (id) => `/analysis/${id}`,
  finding: (id) => `/analysis/findings/${id}`,
}

function go(hit: Schemas['SearchHit']) {
  const to = linkFor[hit.entity_type]?.(hit.id)
  if (!to) return
  open.value = false
  q.value = ''
  void router.push(to)
}

function onKey(e: KeyboardEvent) {
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    active.value = Math.min(active.value + 1, hits.value.length - 1)
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    active.value = Math.max(active.value - 1, 0)
  } else if (e.key === 'Enter') {
    e.preventDefault()
    const hit = hits.value[active.value]
    if (hit) go(hit)
    else if (q.value.trim().length >= 2) {
      open.value = false
      void router.push({ name: 'search', query: { q: q.value.trim() } })
    }
  } else if (e.key === 'Escape') {
    open.value = false
    input.value?.blur()
  }
}

function onBlur() {
  setTimeout(() => (open.value = false), 150)
}

function seeAll() {
  open.value = false
  void router.push({ name: 'search', query: { q: debounced.value.trim() } })
}

onKeyStroke('k', (e) => {
  if (e.ctrlKey || e.metaKey) {
    e.preventDefault()
    input.value?.focus()
  }
})
</script>

<template>
  <div class="relative w-full max-w-lg">
    <Search class="pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2 text-slate-400" aria-hidden="true" />
    <input
      ref="input"
      v-model="q"
      type="search"
      role="combobox"
      aria-label="Search everything"
      :aria-expanded="open && enabled"
      aria-controls="global-search-results"
      placeholder="Search companies, products, findings…  (Ctrl+K)"
      class="input pl-9"
      autocomplete="off"
      @focus="open = true"
      @blur="onBlur"
      @keydown="onKey"
    />
    <div v-if="open && enabled" id="global-search-results" role="listbox" class="absolute z-30 mt-1 max-h-96 w-full overflow-y-auto rounded-md border border-slate-200 bg-white py-1 shadow-lg">
      <p v-if="isFetching && hits.length === 0" class="px-3 py-2 text-sm text-slate-500">Searching…</p>
      <p v-else-if="hits.length === 0" class="px-3 py-2 text-sm text-slate-500">No matches for “{{ debounced }}”.</p>
      <button
        v-for="(h, i) in hits"
        :key="`${h.entity_type}-${h.id}`"
        type="button"
        role="option"
        :aria-selected="i === active"
        class="flex w-full items-start gap-2 px-3 py-2 text-left text-sm"
        :class="i === active ? 'bg-brand-50' : 'hover:bg-slate-50'"
        @mousedown.prevent="go(h)"
        @mouseenter="active = i"
      >
        <span class="mt-0.5 w-24 shrink-0 text-xs text-slate-500">{{ entityLabel(h.entity_type) }}</span>
        <span class="min-w-0">
          <span class="block truncate font-medium text-slate-900">{{ h.title }}</span>
          <span v-if="h.subtitle" class="block truncate text-xs text-slate-500">{{ h.subtitle }}</span>
        </span>
      </button>
      <button v-if="hits.length > 0" type="button" class="block w-full border-t border-slate-100 px-3 py-2 text-left text-xs text-brand-700 hover:bg-slate-50" @mousedown.prevent="seeAll">
        See all results
      </button>
    </div>
  </div>
</template>
