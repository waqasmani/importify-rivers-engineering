<script setup lang="ts" generic="T extends { id: string }">
import { computed } from 'vue'
import { ArrowDown, ArrowUp } from 'lucide-vue-next'
import StateViews from './StateViews.vue'

export interface Column {
  key: string
  label: string
  sortKey?: string
  class?: string
}

const props = withDefaults(
  defineProps<{
    columns: Column[]
    rows: T[]
    loading?: boolean
    error?: string | null
    selectable?: boolean
    sort?: string
    emptyTitle?: string
    emptyMessage?: string
    rowLink?: (row: T) => string | undefined
  }>(),
  { loading: false, error: null, selectable: false, sort: '' },
)
const selected = defineModel<string[]>('selected', { default: () => [] })
const emit = defineEmits<{ sort: [key: string]; retry: [] }>()

const allChecked = computed(() => props.rows.length > 0 && props.rows.every((r) => selected.value.includes(r.id)))
function toggleAll() {
  selected.value = allChecked.value ? [] : props.rows.map((r) => r.id)
}
function toggle(id: string) {
  selected.value = selected.value.includes(id) ? selected.value.filter((x) => x !== id) : [...selected.value, id]
}
function sortState(c: Column): 'asc' | 'desc' | null {
  if (!c.sortKey) return null
  if (props.sort === c.sortKey) return 'asc'
  if (props.sort === `-${c.sortKey}`) return 'desc'
  return null
}
function clickSort(c: Column) {
  if (!c.sortKey) return
  emit('sort', props.sort === c.sortKey ? `-${c.sortKey}` : c.sortKey)
}
function value(row: T, key: string): unknown {
  return (row as Record<string, unknown>)[key]
}
</script>

<template>
  <div class="card overflow-hidden">
    <div class="overflow-x-auto">
      <table class="min-w-full divide-y divide-slate-200">
        <thead class="bg-slate-50">
          <tr>
            <th v-if="selectable" class="w-10 px-3 py-2">
              <input type="checkbox" :checked="allChecked" aria-label="Select all rows" class="rounded border-slate-300" @change="toggleAll" />
            </th>
            <th v-for="c in columns" :key="c.key" scope="col" :class="['table-th', c.class]" :aria-sort="sortState(c) === 'asc' ? 'ascending' : sortState(c) === 'desc' ? 'descending' : undefined">
              <button v-if="c.sortKey" type="button" class="inline-flex items-center gap-1 uppercase hover:text-slate-800" @click="clickSort(c)">
                {{ c.label }}
                <ArrowUp v-if="sortState(c) === 'asc'" class="h-3 w-3" />
                <ArrowDown v-else-if="sortState(c) === 'desc'" class="h-3 w-3" />
              </button>
              <span v-else>{{ c.label }}</span>
            </th>
          </tr>
        </thead>
        <tbody class="divide-y divide-slate-100 bg-white">
          <tr v-for="row in rows" :key="row.id" class="hover:bg-slate-50" :data-testid="`row-${row.id}`">
            <td v-if="selectable" class="px-3 py-2">
              <input type="checkbox" :checked="selected.includes(row.id)" :aria-label="`Select row`" class="rounded border-slate-300" @change="toggle(row.id)" />
            </td>
            <td v-for="c in columns" :key="c.key" :class="['table-td', c.class]">
              <slot :name="`cell-${c.key}`" :row="row" :value="value(row, c.key)">
                <RouterLink v-if="rowLink && c === columns[0] && rowLink(row)" :to="rowLink(row)!" class="font-medium text-brand-700 hover:underline">
                  {{ value(row, c.key) ?? '—' }}
                </RouterLink>
                <template v-else>{{ value(row, c.key) ?? '—' }}</template>
              </slot>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <StateViews v-if="loading && rows.length === 0" kind="loading" />
    <StateViews v-else-if="error" kind="error" :message="error" @retry="emit('retry')" />
    <StateViews v-else-if="!loading && rows.length === 0" kind="empty" :title="emptyTitle" :message="emptyMessage"><slot name="empty" /></StateViews>
  </div>
</template>
