<script setup lang="ts">
// Searchable single-record selector backed by the list endpoints (or the user directory).
import { computed, ref, watch } from 'vue'
import { refDebounced } from '@vueuse/core'
import { useQuery } from '@tanstack/vue-query'
import { X } from 'lucide-vue-next'
import { api } from '@/api/client'

export type PickerType = 'company' | 'contact' | 'supplier' | 'lead' | 'opportunity' | 'product' | 'user'

interface Option {
  id: string
  label: string
  sub?: string
}

const props = withDefaults(
  defineProps<{ type: PickerType; initialLabel?: string | null; placeholder?: string; inputId?: string; exclude?: string[]; disabled?: boolean; invalid?: boolean }>(),
  { initialLabel: null, placeholder: 'Search…', inputId: undefined, exclude: () => [], disabled: false, invalid: false },
)
const model = defineModel<string | null>({ default: null })
const emit = defineEmits<{ select: [option: Option | null] }>()

const endpoints: Record<Exclude<PickerType, 'user'>, string> = {
  company: '/companies',
  contact: '/contacts',
  supplier: '/suppliers',
  lead: '/leads',
  opportunity: '/opportunities',
  product: '/products',
}

function toOption(type: PickerType, r: Record<string, unknown>): Option {
  const s = (k: string) => (typeof r[k] === 'string' ? (r[k] as string) : '')
  switch (type) {
    case 'contact':
      return { id: s('id'), label: s('full_name') || `${s('first_name')} ${s('last_name')}`.trim(), sub: s('email') || s('company_name') }
    case 'lead':
      return { id: s('id'), label: s('title'), sub: s('company_name') }
    case 'product':
      return { id: s('id'), label: s('title'), sub: [s('sku'), s('source_domain')].filter(Boolean).join(' · ') }
    case 'user':
      return { id: s('id'), label: s('display_name'), sub: s('email') }
    default:
      return { id: s('id'), label: s('name'), sub: s('domain') || s('country') }
  }
}

const label = ref(props.initialLabel ?? '')
const text = ref('')
const open = ref(false)
const debounced = refDebounced(text, 200)
const typeKey = computed(() => props.type)

watch(
  () => props.initialLabel,
  (v) => {
    if (v) label.value = v
  },
)

const directory = useQuery({
  queryKey: ['users-directory'],
  queryFn: ({ signal }) => api.get<{ items: Record<string, unknown>[] }>('/users/directory', undefined, signal),
  enabled: computed(() => props.type === 'user'),
  staleTime: 300_000,
})

const results = useQuery({
  queryKey: ['picker', typeKey, debounced],
  queryFn: ({ signal }) =>
    api.get<{ items: Record<string, unknown>[] }>(endpoints[props.type as Exclude<PickerType, 'user'>], { q: debounced.value.trim(), limit: 10 }, signal),
  enabled: computed(() => props.type !== 'user' && open.value),
})

const options = computed<Option[]>(() => {
  let rows: Option[]
  if (props.type === 'user') {
    const q = debounced.value.trim().toLowerCase()
    rows = (directory.data.value?.items ?? []).map((r) => toOption('user', r)).filter((o) => !q || o.label.toLowerCase().includes(q) || (o.sub ?? '').toLowerCase().includes(q))
  } else {
    rows = (results.data.value?.items ?? []).map((r) => toOption(props.type, r))
  }
  return rows.filter((o) => !props.exclude.includes(o.id)).slice(0, 10)
})

// Resolve the label for a preselected id (for example when editing an existing record).
watch(
  [model, () => directory.data.value],
  async ([id]) => {
    if (!id || label.value) return
    if (props.type === 'user') {
      const u = directory.data.value?.items.find((r) => r.id === id)
      if (u) label.value = toOption('user', u).label
      return
    }
    try {
      const r = await api.get<Record<string, unknown>>(`${endpoints[props.type]}/${id}`)
      label.value = toOption(props.type, r).label
    } catch {
      label.value = 'Unknown record'
    }
  },
  { immediate: true },
)

function choose(o: Option) {
  model.value = o.id
  label.value = o.label
  text.value = ''
  open.value = false
  emit('select', o)
}

function clear() {
  model.value = null
  label.value = ''
  emit('select', null)
}

function onBlur() {
  setTimeout(() => (open.value = false), 150)
}
</script>

<template>
  <div class="relative">
    <div v-if="model" class="input flex items-center justify-between gap-2" :class="{ 'opacity-60': disabled }">
      <span class="truncate">{{ label || 'Loading…' }}</span>
      <button v-if="!disabled" type="button" class="rounded p-0.5 text-slate-400 hover:text-slate-700" :aria-label="`Clear ${label}`" @click="clear"><X class="h-3.5 w-3.5" /></button>
    </div>
    <template v-else>
      <input
        :id="inputId"
        v-model="text"
        type="search"
        class="input"
        :class="{ 'border-red-400': invalid }"
        :placeholder="placeholder"
        :disabled="disabled"
        autocomplete="off"
        role="combobox"
        :aria-expanded="open"
        @focus="open = true"
        @input="open = true"
        @blur="onBlur"
      />
      <div v-if="open" class="absolute z-30 mt-1 max-h-64 w-full overflow-y-auto rounded-md border border-slate-200 bg-white py-1 shadow-lg" role="listbox">
        <p v-if="options.length === 0" class="px-3 py-2 text-sm text-slate-500">{{ results.isFetching.value ? 'Searching…' : 'No matches' }}</p>
        <button v-for="o in options" :key="o.id" type="button" role="option" class="block w-full px-3 py-1.5 text-left text-sm hover:bg-slate-50" @mousedown.prevent="choose(o)">
          <span class="block truncate text-slate-900">{{ o.label }}</span>
          <span v-if="o.sub" class="block truncate text-xs text-slate-500">{{ o.sub }}</span>
        </button>
      </div>
    </template>
  </div>
</template>
